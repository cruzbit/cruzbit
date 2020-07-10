// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DNSSeeder returns known peers in response to DNS queries.
type DNSSeeder struct {
	peerStore PeerStorage
	server    *dns.Server
	port      int
	wg        sync.WaitGroup
}

// NewDNSSeeder creates a new DNS seeder given a PeerStorage interface.
func NewDNSSeeder(peerStore PeerStorage, port int) *DNSSeeder {
	return &DNSSeeder{
		peerStore: peerStore,
		port:      port,
		server:    &dns.Server{Addr: "0.0.0.0:" + strconv.Itoa(port), Net: "udp"},
	}
}

func (d *DNSSeeder) handleQuery(m *dns.Msg, externalIP string) {
	for _, q := range m.Question {
		switch q.Qtype {
		case dns.TypeA:
			// get up to 128 peers that we've connected to in the last 48 hours
			addresses, err := d.peerStore.GetSince(128, time.Now().Unix()-(60*60*48))
			if err != nil {
				log.Printf("Error requesting peers from storage: %s\n", err)
				return
			}

			// add ourself
			if len(externalIP) != 0 {
				addresses = append(addresses, externalIP+":"+strconv.Itoa(d.port))
			}

			// shuffle them
			for n := len(addresses); n > 0; n-- {
				randIndex := rand.Intn(n)
				addresses[n-1], addresses[randIndex] = addresses[randIndex], addresses[n-1]
			}

			// return at most 4
			limit := 4
			for i, addr := range addresses {
				if i == limit {
					return
				}
				ip, port, _ := net.SplitHostPort(addr)
				if port != strconv.Itoa(DEFAULT_CRUZBIT_PORT) {
					continue
				}
				rr, err := dns.NewRR(fmt.Sprintf("%s A %s", q.Name, ip))
				if err == nil {
					m.Answer = append(m.Answer, rr)
				}
			}
		}
	}
}

// Run executes the main loop for the DNSSeeder in its own goroutine.
func (d *DNSSeeder) Run() {
	d.wg.Add(1)
	go d.run()
}

func (d *DNSSeeder) run() {
	defer d.wg.Done()

	externalIP, err := determineExternalIP()
	if err != nil {
		log.Println(err)
	}

	handleDnsRequest := func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Compress = false

		switch r.Opcode {
		case dns.OpcodeQuery:
			d.handleQuery(m, externalIP)
		}

		w.WriteMsg(m)
	}

	dns.HandleFunc("cruzbit.", handleDnsRequest)
	log.Printf("Starting DNS server")
	if err := d.server.ListenAndServe(); err != nil {
		log.Println(err)
	}
}

// Shutdown shuts down the DNS seeder server.
func (d *DNSSeeder) Shutdown() {
	log.Println("DNS seeder shutting down...")
	d.server.Shutdown()
	d.wg.Wait()
	log.Println("DNS seeder shutdown")
}

var Seeders = [...]string{
	"66.172.27.47:8831",
	"66.172.12.98:8831", // public.cruzb.it:8831
	"66.117.62.146:8831",
	"dns.cruzb.it:8831",
	"one.cruzb.it:8831",
	"two.cruzb.it:8831",
}

// Query DNS seeders
func dnsQueryForPeers() ([]string, error) {
	var peers []string
	for _, seeder := range Seeders {
		c := dns.Client{}
		m := dns.Msg{}
		m.SetQuestion("client.cruzbit.", dns.TypeA)
		log.Printf("Querying DNS seeder %s\n", seeder)
		r, _, err := c.Exchange(&m, seeder)
		if err != nil {
			log.Printf("Error querying seeder: %s, error: %s\n",
				seeder, err)
			continue
		}
		for _, answer := range r.Answer {
			a := answer.(*dns.A)
			log.Printf("Seeder returned: %s\n", a.A.String())
			peers = append(peers, a.A.String()+":"+strconv.Itoa(DEFAULT_CRUZBIT_PORT))
		}
	}
	return peers, nil
}
