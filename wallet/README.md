# wallet

A wallet is a lightweight client which connects to a peer to receive balance and transaction history information.
It also stores and manages private keys on behalf of the user and can be used to sign and publish transactions with them.

## To install

1. Make sure you have the new Go modules support enabled: `export GO111MODULE=on`
2. `go install github.com/cruzbit/cruzbit/wallet`

The `wallet` application is now in `$HOME/go/bin/wallet`.

## Basic command line arguments

`wallet -walletdb <path to a directory to store wallet data>`

- **walletdb** - This points to a directory on disk to store wallet data including private keys. Keys will be encrypted at-rest using a passphrase you will be prompted to enter.

## Other options

- **peer** - Specifies the address of a peer to talk to for balance and transaction history information. It will also publish newly signed transactions to this peer. By default, it connects to `127.0.0.1:8831`.

## Usage

You should only connect the wallet to a client peer you trust. A bad client can misbehave in all sorts of ways that could confuse your wallet and trick you into making transactions you otherwise wouldn't. You also expose which public keys you control to the peer.

The prompt should be mostly self-documenting. Use the `newkey` command to generate a public/private key pair. The displayed public key can be used as the `-pubkey` argument to the [client program.](https://github.com/cruzbit/cruzbit/tree/master/client)
