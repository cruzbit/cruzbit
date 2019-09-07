// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

// PeerStorage is an interface for storing peer addresses and information about their connectivity.
type PeerStorage interface {
	// Store stores a peer address. Returns true if the peer was newly added to storage.
	Store(addr string) (bool, error)

	// Get returns some peers for us to attempt to connect to.
	Get(count int) ([]string, error)

	// GetSince returns some peers to tell others about last active less than "when" ago.
	GetSince(count int, when int64) ([]string, error)

	// Delete is called to explicitly remove a peer address from storage.
	Delete(addr string) error

	// OnConnectAttempt is called prior to attempting to connect to the peer.
	OnConnectAttempt(addr string) error

	// OnConnectSuccess is called upon successful handshake with the peer.
	OnConnectSuccess(addr string) error

	// OnConnectFailure is called upon connection failure.
	OnConnectFailure(addr string) error

	// OnDisconnect is called upon disconnection.
	OnDisconnect(addr string) error
}
