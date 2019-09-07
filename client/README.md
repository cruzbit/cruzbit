![cruzbit_logo_v1 half](https://user-images.githubusercontent.com/51346587/61192334-61417480-a668-11e9-94a6-bdbc43243600.png)

# client

A client is a fully validating and mining peer-to-peer node in the cruzbit network.

## To install

1. Make sure you have the new Go modules support enabled: `export GO111MODULE=on`
2. `go install github.com/cruzbit/cruzbit/client`

The `client` application is now in `$HOME/go/bin/client`.

## Basic command line arguments

`client -pubkey <base64 encoded public key> -datadir <somewhere to store data>`

- **pubkey** - This is a public key which receives your node's mining rewards. You can create one with the [wallet software](https://github.com/cruzbit/cruzbit/tree/master/wallet).
- **datadir** - This points to a directory on disk to store block chain and ledger data. It will be created if it doesn't exist.

## What will the client do?

With the above specified options, the client will: 

- Listen on TCP port 8831 for new peer connections (up to 128.)
- Attempt to discover peers and connect to them (up to 8.)
- Discover peers using the [DNS protocol](https://en.wikipedia.org/wiki/Domain_Name_System) used with hardcoded seed nodes.
- Discover peers via [IRC](https://en.wikipedia.org/wiki/Internet_Relay_Chat) and advertise itself as available for inbound connection (if it determines this is possible.)
- Attempt to mine new blocks and share them with connected peers.
- Validate and share new blocks and transactions with peers.

## Other options
- **memo** - A memo to include in newly mined blocks.
- **port** - By default, cruzbit nodes accept connections on TCP port 8831.
- **peer** - Address of a peer to connect to. Useful for wallets and testing.
- **upnp** - If specified, attempt to forward the cruzbit port on your router with [UPnP](https://en.wikipedia.org/wiki/Universal_Plug_and_Play).
- **dnsseed** - If specified, run a DNS server to allow others to find peers on UDP port 8831.
- **compress** - If specified, compress blocks on disk with [LZ4](https://en.wikipedia.org/wiki/LZ4_(compression_algorithm)). Can safely be toggled.
- **numminers** - Number of miner threads to run. Default is 1.
- **noirc** - Disable use of IRC for peer discovery. Default is true.
- **noaccept** - Disable inbound peer connections.
- **keyfile** - Path to a file containing public keys to use when mining. Keys will be used randomly.
- **prune** - If specified, only the last 2016 blocks (roughly 2 weeks) worth of transaction and public key transaction indices are stored in the ledger. This only impacts a wallet's ability to query for history older than that. It can still query for current balances of all public keys.
- **tlscert** - Path to a file containing a PEM-encoded X.509 certificate to use with TLS.
- **tlskey** - Path to a file containing a PEM-encoded private key to use with TLS.
- **inlimit** - Limit for the number of inbound peer connections. Default is 128.
- **banlist** - Path to a file containing a list of banned host addresses.
