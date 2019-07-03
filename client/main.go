// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	. "github.com/cruzbit/cruzbit"
	"golang.org/x/crypto/ed25519"
)

// A peer node in the cruzbit network
func main() {
	rand.Seed(time.Now().UnixNano())

	// flags
	pubKeyPtr := flag.String("pubkey", "", "A public key which receives newly mined block rewards")
	dataDirPtr := flag.String("datadir", "", "Path to a directory to save block chain data")
	memoPtr := flag.String("memo", "", "A memo to include in newly mined blocks")
	portPtr := flag.Int("port", DEFAULT_CRUZBIT_PORT, "Port to listen for incoming peer connections")
	peerPtr := flag.String("peer", "", "Address of a peer to connect to")
	upnpPtr := flag.Bool("upnp", false, "Attempt to forward the cruzbit port on your router with UPnP")
	dnsSeedPtr := flag.Bool("dnsseed", false, "Run a DNS server to allow others to find peers")
	compressPtr := flag.Bool("compress", false, "Compress blocks on disk with lz4")
	numMinersPtr := flag.Int("numminers", 1, "Number of miners to run")
	noIrcPtr := flag.Bool("noirc", false, "Disable use of IRC for peer discovery")
	noAcceptPtr := flag.Bool("noaccept", false, "Disable inbound peer connections")
	prunePtr := flag.Bool("prune", false, "Prune transaction and public key transaction indices")
	keyFilePtr := flag.String("keyfile", "", "Path to a file containing public keys to use when mining")
	tlsCertPtr := flag.String("tlscert", "", "Path to a file containing a PEM-encoded X.509 certificate to use with TLS")
	tlsKeyPtr := flag.String("tlskey", "", "Path to a file containing a PEM-encoded EC key to use with TLS")
	inLimitPtr := flag.Int("inlimit", MAX_INBOUND_PEER_CONNECTIONS, "Limit for the number of inbound peer connections.")
	flag.Parse()

	if len(*dataDirPtr) == 0 {
		log.Fatal("-datadir argument required")
	}
	if len(*tlsCertPtr) != 0 && len(*tlsKeyPtr) == 0 {
		log.Fatal("-tlskey argument missing")
	}
	if len(*tlsCertPtr) == 0 && len(*tlsKeyPtr) != 0 {
		log.Fatal("-tlscert argument missing")
	}

	var pubKeys []ed25519.PublicKey
	if *numMinersPtr > 0 {
		if len(*pubKeyPtr) == 0 && len(*keyFilePtr) == 0 {
			log.Fatal("-pubkey or -keyfile argument required to receive newly mined block rewards")
		}
		if len(*pubKeyPtr) != 0 && len(*keyFilePtr) != 0 {
			log.Fatal("Specify only one of -pubkey or -keyfile but not both")
		}
		var err error
		pubKeys, err = loadPublicKeys(*pubKeyPtr, *keyFilePtr)
		if err != nil {
			log.Fatal(err)
		}
	}

	// load genesis block
	genesisBlock := new(Block)
	if err := json.Unmarshal([]byte(GenesisBlockJson), genesisBlock); err != nil {
		log.Fatal(err)
	}

	genesisID, err := genesisBlock.ID()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Starting up...")
	log.Printf("Genesis block ID: %s\n", genesisID)

	// instantiate storage
	blockStore, err := NewBlockStorageDisk(
		filepath.Join(*dataDirPtr, "blocks"),
		filepath.Join(*dataDirPtr, "headers.db"),
		false, // not read-only
		*compressPtr,
	)
	if err != nil {
		log.Fatal(err)
	}

	// instantiate the ledger
	ledger, err := NewLedgerDisk(filepath.Join(*dataDirPtr, "ledger.db"),
		false, // not read-only
		*prunePtr,
		blockStore)
	if err != nil {
		blockStore.Close()
		log.Fatal(err)
	}

	// instantiate peer storage
	peerStore, err := NewPeerStorageDisk(filepath.Join(*dataDirPtr, "peers.db"))
	if err != nil {
		ledger.Close()
		blockStore.Close()
		log.Fatal(err)
	}

	// instantiate the transaction queue
	txQueue := NewTransactionQueueMemory(ledger)

	// create and run the processor
	processor := NewProcessor(genesisID, blockStore, txQueue, ledger)
	processor.Run()

	// process the genesis block
	if err := processor.ProcessBlock(genesisID, genesisBlock, ""); err != nil {
		processor.Shutdown()
		peerStore.Close()
		ledger.Close()
		blockStore.Close()
		log.Fatal(err)
	}

	var miners []*Miner
	var hashrateMonitor *HashrateMonitor
	if *numMinersPtr > 0 {
		hashUpdateChan := make(chan int64, *numMinersPtr)
		// create and run miners
		for i := 0; i < *numMinersPtr; i++ {
			miner := NewMiner(pubKeys, *memoPtr, blockStore, txQueue, ledger, processor, hashUpdateChan, i)
			miners = append(miners, miner)
			miner.Run()
		}
		// print hashrate updates
		hashrateMonitor = NewHashrateMonitor(hashUpdateChan)
		hashrateMonitor.Run()
	} else {
		log.Println("Mining is currently disabled")
	}

	// start a dns server
	var seeder *DNSSeeder
	if *dnsSeedPtr {
		seeder = NewDNSSeeder(peerStore, *portPtr)
		seeder.Run()
	}

	// enable port forwarding (accept must also be enabled)
	var myExternalIP string
	if *upnpPtr == true && *noAcceptPtr == false {
		log.Printf("Enabling forwarding for port %d...\n", *portPtr)
		var ok bool
		var err error
		if myExternalIP, ok, err = HandlePortForward(uint16(*portPtr), true); err != nil || !ok {
			log.Printf("Failed to enable forwarding: %s\n", err)
		} else {
			log.Println("Successfully enabled forwarding")
		}
	}

	// manage peer connections
	peerManager := NewPeerManager(genesisID, peerStore, blockStore, ledger, processor, txQueue,
		*dataDirPtr, myExternalIP, *peerPtr, *tlsCertPtr, *tlsKeyPtr,
		*portPtr, *inLimitPtr, !*noAcceptPtr, !*noIrcPtr)
	peerManager.Run()

	// shutdown on ctrl-c
	c := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(c, os.Interrupt)

	go func() {
		defer close(done)
		<-c

		log.Println("Shutting down...")

		if len(myExternalIP) != 0 {
			// disable port forwarding
			log.Printf("Disabling forwarding for port %d...", *portPtr)
			if _, ok, err := HandlePortForward(uint16(*portPtr), false); err != nil || !ok {
				log.Printf("Failed to disable forwarding: %s", err)
			} else {
				log.Println("Successfully disabled forwarding")
			}
		}

		// shut everything down now
		peerManager.Shutdown()
		if seeder != nil {
			seeder.Shutdown()
		}
		for _, miner := range miners {
			miner.Shutdown()
		}
		if hashrateMonitor != nil {
			hashrateMonitor.Shutdown()
		}
		processor.Shutdown()

		// close storage
		if err := peerStore.Close(); err != nil {
			log.Println(err)
		}
		if err := ledger.Close(); err != nil {
			log.Println(err)
		}
		if err := blockStore.Close(); err != nil {
			log.Println(err)
		}
	}()

	log.Println("Client started")
	<-done
	log.Println("Exiting")
}

func loadPublicKeys(pubKeyEncoded, keyFile string) ([]ed25519.PublicKey, error) {
	var pubKeysEncoded []string
	var pubKeys []ed25519.PublicKey

	if len(pubKeyEncoded) != 0 {
		pubKeysEncoded = append(pubKeysEncoded, pubKeyEncoded)
	} else {
		file, err := os.Open(keyFile)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			pubKeysEncoded = append(pubKeysEncoded, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		if len(pubKeysEncoded) == 0 {
			return nil, fmt.Errorf("No public keys found in '%s'", keyFile)
		}
	}

	for _, pubKeyEncoded = range pubKeysEncoded {
		pubKeyBytes, err := base64.StdEncoding.DecodeString(pubKeyEncoded)
		if len(pubKeyBytes) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("Invalid public key: %s\n", pubKeyEncoded)
		}
		if err != nil {
			return nil, err
		}
		pubKeys = append(pubKeys, ed25519.PublicKey(pubKeyBytes))
	}
	return pubKeys, nil
}
