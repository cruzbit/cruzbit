// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"io"
	"log"
	mrand "math/rand"
	"regexp"
	"strconv"
	"sync"

	irc "github.com/thoj/go-ircevent"
)

const Server = "irc.freenode.net:7000"

// IRC is used for bootstrapping the network for now.
// It primarily exists as a backup to our current limited set of DNS seeders.
type IRC struct {
	conn *irc.Connection
	wg   sync.WaitGroup
}

// NewIRC returns a new IRC bootstrapper.
func NewIRC() *IRC {
	return &IRC{}
}

// Connect connects the IRC bootstrapper to the IRC network.
// port is our local cruzbit port. If it's set to 0 we won't be used for inbound connections.
func (i *IRC) Connect(genesisID BlockID, port int, addrChan chan<- string) error {
	nick, err := generateRandomNick()
	if err != nil {
		return err
	}

	numReg, err := regexp.Compile("[^0-9]+")
	if err != nil {
		return err
	}

	i.conn = irc.IRC(nick, strconv.Itoa(port))
	i.conn.UseTLS = true
	i.conn.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	var channel string
	i.conn.AddCallback("001", func(e *irc.Event) {
		n := mrand.Intn(10)
		channel = generateChannelName(genesisID, n)
		log.Printf("Joining channel %s\n", channel)
		i.conn.Join(channel)
	})
	i.conn.AddCallback("366", func(e *irc.Event) {
		log.Printf("Joined channel %s\n", channel)
		i.conn.Who(channel)
	})
	i.conn.AddCallback("352", func(e *irc.Event) {
		if e.Arguments[5] == nick {
			return
		}
		port := numReg.ReplaceAllString(e.Arguments[2], "")
		host := e.Arguments[3]
		if len(port) != 0 && port != "0" {
			peer := host + ":" + port
			addrChan <- peer
		}
	})
	i.conn.AddCallback("JOIN", func(e *irc.Event) {
		if e.Nick == nick {
			return
		}
		port := numReg.ReplaceAllString(e.User, "")
		host := e.Host
		if len(port) != 0 && port != "0" {
			peer := host + ":" + port
			addrChan <- peer
		}
	})

	return i.conn.Connect(Server)
}

// Run executes the IRC bootstrapper's main loop in its own goroutine.
func (i *IRC) Run() {
	i.wg.Add(1)
	go func() {
		defer i.wg.Done()
		i.conn.Loop()
	}()
}

// Shutdown stops the IRC bootstrapper.
func (i *IRC) Shutdown() {
	log.Println("IRC shutting down...")
	i.conn.Quit()
	i.wg.Wait()
	log.Println("IRC shutdown")
}

func generateRandomNick() (string, error) {
	nickBytes := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, nickBytes); err != nil {
		return "", err
	}
	return "cb" + hex.EncodeToString(nickBytes)[2:], nil
}

func generateChannelName(genesisID BlockID, n int) string {
	g := genesisID.String()
	return "#cruzbit-" + g[len(g)-8:] + "-" + strconv.Itoa(n)
}
