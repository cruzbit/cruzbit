// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/GeertJohan/go.rice"
	"github.com/glendc/go-external-ip"
)

// PeerManager manages incoming and outgoing peer connections on behalf of the client.
// It also manages finding peers to connect to.
type PeerManager struct {
	genesisID         BlockID
	peerStore         PeerStorage
	blockStore        BlockStorage
	ledger            Ledger
	processor         *Processor
	txQueue           TransactionQueue
	blockQueue        *BlockQueue
	dataDir           string
	myIP              string
	peer              string
	certPath          string
	keyPath           string
	port              int
	inboundLimit      int
	accept            bool
	accepting         bool
	irc               bool
	dnsseed           bool
	inPeers           map[string]*Peer
	inPeerCountByHost map[string]int
	outPeers          map[string]*Peer
	inPeersLock       sync.RWMutex
	outPeersLock      sync.RWMutex
	addrChan          chan string
	peerNonce         string
	open              bool
	privateIPBlocks   []*net.IPNet
	server            *http.Server
	cancelFunc        context.CancelFunc
	shutdownChan      chan bool
	wg                sync.WaitGroup
}

// NewPeerManager returns a new PeerManager instance.
func NewPeerManager(
	genesisID BlockID, peerStore PeerStorage, blockStore BlockStorage,
	ledger Ledger, processor *Processor, txQueue TransactionQueue,
	dataDir, myExternalIP, peer, certPath, keyPath string,
	port, inboundLimit int, accept, irc, dnsseed bool) *PeerManager {

	// compute and save these
	var privateIPBlocks []*net.IPNet
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, _ := net.ParseCIDR(cidr)
		privateIPBlocks = append(privateIPBlocks, block)
	}

	// server to listen for and handle incoming secure WebSocket connections
	server := &http.Server{
		Addr:         "0.0.0.0:" + strconv.Itoa(port),
		TLSConfig:    tlsServerConfig, // from tls.go
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return &PeerManager{
		genesisID:         genesisID,
		peerStore:         peerStore,
		blockStore:        blockStore,
		ledger:            ledger,
		processor:         processor,
		txQueue:           txQueue,
		blockQueue:        NewBlockQueue(),
		dataDir:           dataDir,
		myIP:              myExternalIP, // set if upnp was enabled and successful
		peer:              peer,
		certPath:          certPath,
		keyPath:           keyPath,
		port:              port,
		inboundLimit:      inboundLimit,
		accept:            accept,
		irc:               irc,
		dnsseed:           dnsseed,
		inPeers:           make(map[string]*Peer),
		inPeerCountByHost: make(map[string]int),
		outPeers:          make(map[string]*Peer),
		addrChan:          make(chan string, 10000),
		peerNonce:         strconv.Itoa(int(rand.Int31())),
		privateIPBlocks:   privateIPBlocks,
		server:            server,
		shutdownChan:      make(chan bool),
	}
}

// Run executes the PeerManager's main loop in its own goroutine.
// It determines our connectivity and manages sourcing peer addresses from seed sources
// as well as maintaining full outbound connections and accepting inbound connections.
func (p *PeerManager) Run() {
	p.wg.Add(1)
	go p.run()
}

func (p *PeerManager) run() {
	defer p.wg.Done()

	// parent context for all peer connection requests
	ctx, cancel := context.WithCancel(context.Background())
	p.cancelFunc = cancel

	// determine external ip
	myExternalIP, err := determineExternalIP()
	if err != nil {
		log.Printf("Error determining external IP: %s\n", err)
	} else {
		log.Printf("My external IP address is: %s\n", myExternalIP)
		if len(p.myIP) != 0 {
			// if upnp enabled make sure the address returned matches the outside view
			p.open = myExternalIP == p.myIP
		} else {
			// if no upnp see if any local routable ip matches the outside view
			p.open, err = haveLocalIPMatch(myExternalIP)
			if err != nil {
				log.Printf("Error checking for local IP match: %s\n", err)
			}
		}
		p.myIP = myExternalIP
	}

	var irc *IRC
	if len(p.peer) != 0 {
		// store the explicitly specified outbound peer
		if _, err := p.peerStore.Store(p.peer); err != nil {
			log.Printf("Error saving peer: %s, address: %s\n", err, p.peer)
		}
	} else {
		// query dns seeds for peers
		addresses, err := dnsQueryForPeers()
		if err != nil {
			log.Printf("Error from DNS query: %s\n", err)
		} else {
			for _, addr := range addresses {
				log.Printf("Got peer address from DNS: %s\n", addr)
				p.addrChan <- addr
			}
		}

		// handle IRC seeding
		if p.irc == true {
			port := p.port
			if !p.open || !p.accept {
				// don't advertise ourself as available for inbound connections
				port = 0
			}
			irc = NewIRC()
			if err := irc.Connect(p.genesisID, port, p.addrChan); err != nil {
				log.Println(err)
			} else {
				irc.Run()
			}
		}
	}

	// handle listening for inbound peers
	p.listenForPeers(ctx)

	// try connecting to some saved peers
	p.connectToPeers(ctx)

	// try connecting out to peers every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// main loop
	for {
		select {
		case addr := <-p.addrChan:
			// parse, resolve and validate the address
			host, port, err := p.parsePeerAddress(addr)
			if err != nil {
				continue
			}

			// store the peer
			resolvedAddr := host + ":" + port
			ok, err := p.peerStore.Store(resolvedAddr)
			if err != nil {
				log.Printf("Error saving peer: %s, address: %s\n", err, resolvedAddr)
				continue
			}
			if !ok {
				// we already knew about this peer address
				continue
			}
			log.Printf("Discovered new peer: %s\n", resolvedAddr)

			// try connecting to some saved peers
			p.connectToPeers(ctx)

		case <-ticker.C:
			outCount, inCount := p.outboundPeerCount(), p.inboundPeerCount()
			log.Printf("Have %d outbound connections and %d inbound connections\n",
				outCount, inCount)

			// handle listening for inbound peers
			p.listenForPeers(ctx)

			if p.dnsseed && rand.Intn(2) == 1 {
				// drop a peer so we can try another
				p.dropRandomPeer()
			}

			// periodically try connecting to some saved peers
			p.connectToPeers(ctx)

		case _, ok := <-p.shutdownChan:
			if !ok {
				log.Println("Peer manager shutting down...")

				if irc != nil {
					// shutdown irc
					irc.Shutdown()
				}

				// shutdown http server
				p.server.Shutdown(context.Background())
				return
			}
		}
	}
}

// Shutdown stops the peer manager synchronously.
func (p *PeerManager) Shutdown() {
	if p.cancelFunc != nil {
		// cancel any connection requests in progress
		p.cancelFunc()
	}
	// shutdown the main loop
	close(p.shutdownChan)
	p.wg.Wait()

	// shutdown all connected peers
	var peers []*Peer
	func() {
		p.outPeersLock.RLock()
		defer p.outPeersLock.RUnlock()
		for _, peer := range p.outPeers {
			peers = append(peers, peer)
		}
	}()
	func() {
		p.inPeersLock.RLock()
		defer p.inPeersLock.RUnlock()
		for _, peer := range p.inPeers {
			peers = append(peers, peer)
		}
	}()
	for _, peer := range peers {
		peer.Shutdown()
	}

	log.Println("Peer manager shutdown")
}

func (p *PeerManager) inboundPeerCount() int {
	p.inPeersLock.RLock()
	defer p.inPeersLock.RUnlock()
	return len(p.inPeers)
}

func (p *PeerManager) outboundPeerCount() int {
	p.outPeersLock.RLock()
	defer p.outPeersLock.RUnlock()
	return len(p.outPeers)
}

// Try connecting to some recent peers
func (p *PeerManager) connectToPeers(ctx context.Context) error {
	if len(p.peer) != 0 {
		if p.outboundPeerCount() != 0 {
			// only connect to the explicitly requested peer once
			return nil
		}

		// try reconnecting to the explicit peer
		log.Printf("Attempting to connect to: %s\n", p.peer)
		if statusCode, _, err := p.connect(ctx, p.peer); err != nil {
			log.Printf("Error connecting to peer: %s, status code: %d\n", err, statusCode)
			return err
		}
		log.Printf("Connected to peer: %s\n", p.peer)
		return nil
	}

	// are we syncing?
	ibd, _, err := IsInitialBlockDownload(p.ledger, p.blockStore)
	if err != nil {
		return err
	}

	var want int
	if ibd {
		// only connect to 1 peer until we're synced.
		// if this client is a bad actor we'll find out about the real
		// chain as soon as we think we're done with them and find more peers
		want = 1
	} else {
		// otherwise try to keep us maximally connected
		want = MAX_OUTBOUND_PEER_CONNECTIONS
	}

	count := p.outboundPeerCount()
	need := want - count
	if need <= 0 {
		return nil
	}

	tried := make(map[string]bool)

	log.Printf("Have %d outbound connections, want %d. Trying some peer addresses now\n",
		count, want)

	// try to satisfy desired outbound peer count
	for need > 0 {
		addrs, err := p.peerStore.Get(need)
		if err != nil {
			return err
		}
		if len(addrs) == 0 {
			// no more attempts possible at the moment
			log.Println("No more peer addresses to try right now")
			return nil
		}
		for _, addr := range addrs {
			if tried[addr] {
				// we already tried this peer address.
				// this shouldn't really be necessary if peer storage is respecting
				// proper retry intervals but it doesn't hurt to be safe
				log.Printf("Already tried to connect to %s this time, will try again later\n",
					addr)
				return nil
			}
			tried[addr] = true
			log.Printf("Attempting to connect to: %s\n", addr)
			if statusCode, _, err := p.connect(ctx, addr); err != nil {
				log.Printf("Error connecting to peer: %s, status code: %d\n", err, statusCode)
				if ctx.Err() != nil {
					// quit trying connections if the parent context was cancelled
					return ctx.Err()
				}
			} else {
				log.Printf("Connected to peer: %s\n", addr)
			}
		}
		count = p.outboundPeerCount()
		need = want - count
	}

	log.Printf("Have %d outbound connections. Done trying new peer addresses\n", count)
	return nil
}

// Connect to a peer
func (p *PeerManager) connect(ctx context.Context, addr string) (int, *Peer, error) {
	peer := NewPeer(nil, p.genesisID, p.peerStore, p.blockStore, p.ledger, p.processor, p.txQueue, p.blockQueue, p.addrChan)

	if ok := p.addToOutboundSet(addr, peer); !ok {
		return 0, nil, fmt.Errorf("Too many peer connections")
	}

	var myAddress string
	if p.accepting && p.open {
		// advertise ourself as open
		myAddress = p.myIP + ":" + strconv.Itoa(p.port)
	}

	// connect to the peer
	statusCode, err := peer.Connect(ctx, addr, p.peerNonce, myAddress)
	if err != nil {
		p.removeFromOutboundSet(addr)
		return statusCode, nil, err
	}

	peer.OnClose(func() {
		p.removeFromOutboundSet(addr)
	})
	peer.Run()

	return statusCode, peer, nil
}

// Check to see if it's time to start accepting connections and do so if necessary
func (p *PeerManager) listenForPeers(ctx context.Context) bool {
	if p.accept == false {
		return false
	}
	if p.accepting == true {
		return true
	}

	// don't accept new connections while we're syncing
	ibd, _, err := IsInitialBlockDownload(p.ledger, p.blockStore)
	if err != nil {
		log.Fatal(err)
	}
	if ibd {
		log.Println("We're still syncing. Not accepting new connections yet")
		return false
	}

	p.accepting = true
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.acceptConnections()
	}()

	// give us some time to generate a certificate and start listening
	// so we can correctly report connectivity to outbound peers
	select {
	case <-time.After(5 * time.Second):
		break
	case <-ctx.Done():
		break
	}

	if !p.open {
		// if we don't yet think we're open try connecting to ourself to see if maybe we are.
		// if the user manually forwarded a port on their router this is when we'd find out.
		log.Println("Checking to see if we're open for public inbound connections")
		myAddr := p.myIP + ":" + strconv.Itoa(p.port)
		if _, err := p.peerStore.Store(myAddr); err == nil {
			statusCode, peer, err := p.connect(ctx, myAddr)
			if statusCode == http.StatusLoopDetected {
				p.open = true
			}
			if err == nil && peer != nil {
				peer.Shutdown()
			}
		}
		if p.open {
			log.Println("Open for public inbound connections")
		} else {
			log.Println("Not open for public inbound connections")
		}
	}
	return true
}

// Accept incoming peer connections
func (p *PeerManager) acceptConnections() {
	// handle incoming connection upgrade requests
	peerHandler := func(w http.ResponseWriter, r *http.Request) {
		// check the connection limit for this peer address
		if !p.checkHostConnectionLimit(r.RemoteAddr) {
			log.Printf("Too many connections from this peer's host: %s\n", r.RemoteAddr)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// check the peer nonce
		theirNonce := r.Header.Get("Cruzbit-Peer-Nonce")
		if theirNonce == p.peerNonce {
			log.Printf("Received connection with our own nonce")
			// write back error reply
			w.WriteHeader(http.StatusLoopDetected)
			return
		}

		// if they set their address it means they think they are open
		theirAddress := r.Header.Get("Cruzbit-Peer-Address")
		if len(theirAddress) != 0 {
			// parse, resolve and validate the address
			host, port, err := p.parsePeerAddress(theirAddress)
			if err != nil {
				log.Printf("Peer address in header is invalid: %s\n", err)

				// don't save it
				theirAddress = ""
			} else {
				// save the resolved address
				theirAddress = host + ":" + port

				// see if we're already connected outbound to them
				if p.existsInOutboundSet(theirAddress) {
					log.Printf("Already connected to %s, dropping inbound connection",
						theirAddress)
					// write back error reply
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}

				// save their address for later use
				if _, err := p.peerStore.Store(theirAddress); err != nil {
					log.Printf("Error saving peer: %s, address: %s\n", err, theirAddress)
				}
			}
		}

		// accept the new websocket
		conn, err := PeerUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("Upgrade:", err)
			return
		}

		peer := NewPeer(conn, p.genesisID, p.peerStore, p.blockStore, p.ledger, p.processor, p.txQueue, p.blockQueue, p.addrChan)

		if ok := p.addToInboundSet(r.RemoteAddr, peer); !ok {
			// TODO: tell the peer why
			peer.Shutdown()
			return
		}

		addr := conn.RemoteAddr().String()
		log.Printf("New peer connection from: %s", addr)
		peer.OnClose(func() {
			p.removeFromInboundSet(addr)
		})
		peer.Run()
	}

	var certPath, keyPath string = p.certPath, p.keyPath
	if len(certPath) == 0 {
		// generate new certificate and key for tls on each run
		log.Println("Generating TLS certificate and key")
		var err error
		certPath, keyPath, err = generateSelfSignedCertAndKey(p.dataDir)
		if err != nil {
			log.Println(err)
			return
		}
	}

	// listen for websocket requests using the genesis block ID as the handler pattern
	http.HandleFunc("/"+p.genesisID.String(), peerHandler)

	// serve a status page
	http.Handle("/", http.FileServer(rice.MustFindBox("html").HTTPBox()))

	log.Println("Listening for new peer connections")
	if err := p.server.ListenAndServeTLS(certPath, keyPath); err != nil {
		log.Println(err)
	}
}

// Helper to add peers to the outbound set if they'll fit
func (p *PeerManager) addToOutboundSet(addr string, peer *Peer) bool {
	p.outPeersLock.Lock()
	defer p.outPeersLock.Unlock()
	if len(p.outPeers) == MAX_OUTBOUND_PEER_CONNECTIONS {
		// too many connections
		return false
	}
	if _, ok := p.outPeers[addr]; ok {
		// already connected
		return false
	}
	p.outPeers[addr] = peer
	log.Printf("Outbound peer count: %d\n", len(p.outPeers))
	return true
}

// Helper to add peers to the inbound set if they'll fit
func (p *PeerManager) addToInboundSet(addr string, peer *Peer) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("Error parsing IP %s: %s\n", addr, err)
		return false
	}
	p.inPeersLock.Lock()
	defer p.inPeersLock.Unlock()
	if len(p.inPeers) == p.inboundLimit {
		// too many connections
		return false
	}
	if _, ok := p.inPeers[addr]; ok {
		// already connected
		return false
	}
	// update the count for this IP
	count, ok := p.inPeerCountByHost[host]
	if !ok {
		p.inPeerCountByHost[host] = 1
	} else {
		p.inPeerCountByHost[host] = count + 1
	}
	p.inPeers[addr] = peer
	log.Printf("Inbound peer count: %d\n", len(p.inPeers))
	return true
}

// Returns false if this host has too many inbound connections already.
func (p *PeerManager) checkHostConnectionLimit(addr string) bool {
	// split host and port
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		log.Printf("Error parsing host from %s: %s\n", addr, err)
		return false
	}
	// resolve the host to an ip
	ips, err := net.LookupIP(host)
	if err != nil {
		log.Printf("Unable to resolve IP address for: %s, error: %s", host, err)
		return false
	}
	if len(ips) == 0 {
		log.Printf("No IP address found for peer address: %s", addr)
		return false
	}
	// filter out local networks
	for _, block := range p.privateIPBlocks {
		if block.Contains(ips[0]) {
			// no limit for loopback peers
			return true
		}
	}

	p.inPeersLock.Lock()
	defer p.inPeersLock.Unlock()
	count, ok := p.inPeerCountByHost[host]
	if !ok {
		return true
	}
	return count < MAX_INBOUND_PEER_CONNECTIONS_FROM_SAME_HOST
}

// Helper to check if a peer address exists in the outbound set
func (p *PeerManager) existsInOutboundSet(addr string) bool {
	p.outPeersLock.RLock()
	defer p.outPeersLock.RUnlock()
	_, ok := p.outPeers[addr]
	return ok
}

// Helper to remove peers from the outbound set
func (p *PeerManager) removeFromOutboundSet(addr string) {
	p.outPeersLock.Lock()
	defer p.outPeersLock.Unlock()
	delete(p.outPeers, addr)
	log.Printf("Outbound peer count: %d\n", len(p.outPeers))
}

// Helper to remove peers from the inbound set
func (p *PeerManager) removeFromInboundSet(addr string) {
	// we parsed this address on the way in so an error isn't possible
	host, _, _ := net.SplitHostPort(addr)
	p.inPeersLock.Lock()
	defer p.inPeersLock.Unlock()
	delete(p.inPeers, addr)
	if count, ok := p.inPeerCountByHost[host]; ok {
		count--
		if count == 0 {
			delete(p.inPeerCountByHost, host)
		} else {
			p.inPeerCountByHost[host] = count
		}
	}
	log.Printf("Inbound peer count: %d\n", len(p.inPeers))
}

// Drop a random peer. Used by seeders
func (p *PeerManager) dropRandomPeer() {
	var peers []*Peer
	func() {
		p.outPeersLock.RLock()
		defer p.outPeersLock.RUnlock()
		for _, peer := range p.outPeers {
			peers = append(peers, peer)
		}
	}()
	if len(peers) > 0 {
		peer := peers[rand.Intn(len(peers))]
		log.Printf("Dropping random peer: %s\n", peer.conn.RemoteAddr())
		peer.Shutdown()
	}
}

// Parse, resolve and validate peer addreses
func (p *PeerManager) parsePeerAddress(addr string) (string, string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("Malformed peer address: %s", addr)
	}

	// sanity check port
	portInt, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return "", "", fmt.Errorf("Invalid port in peer address: %s", addr)
	}

	// resolve the host to an ip
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", "", fmt.Errorf("Unable to resolve IP address for: %s, error: %s", host, err)
	}
	if len(ips) == 0 {
		return "", "", fmt.Errorf("No IP address found for peer address: %s", addr)
	}

	// don't accept ourself
	if p.myIP == ips[0].String() && p.port == int(portInt) {
		return "", "", fmt.Errorf("Peer address is ours: %s", addr)
	}

	// filter out local networks
	for _, block := range p.privateIPBlocks {
		if block.Contains(ips[0]) {
			return "", "", fmt.Errorf("IP is in local address space: %s", ips[0])
		}
	}

	return ips[0].String(), port, nil
}

// Do any of our local IPs match our external IP?
func haveLocalIPMatch(externalIP string) (bool, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false, err
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.String() == externalIP {
				return true, nil
			}
		}
	}
	return false, nil
}

// Determine external IP address
func determineExternalIP() (string, error) {
	consensus := externalip.DefaultConsensus(nil, nil)
	ip, err := consensus.ExternalIP()
	if err != nil {
		return "", err
	}
	return ip.String(), nil
}

// IsInitialBlockDownload returns true if it appears we're still syncing the block chain.
func IsInitialBlockDownload(ledger Ledger, blockStore BlockStorage) (bool, int64, error) {
	tipID, tipHeader, _, err := getChainTipHeader(ledger, blockStore)
	if err != nil {
		return false, 0, err
	}
	if tipID == nil {
		return true, 0, nil
	}
	if tipHeader == nil {
		return true, 0, nil
	}
	if CheckpointsEnabled && tipHeader.Height < LatestCheckpointHeight {
		return true, tipHeader.Height, nil
	}
	return tipHeader.Time < (time.Now().Unix() - MAX_TIP_AGE), tipHeader.Height, nil
}
