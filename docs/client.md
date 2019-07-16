# Client

## Building the Client

To build the latest wallet binaries from master, simply invoke the Go toolchain like so:

```
$ export GO111MODULE=on
$ go get -v github.com/cruzbit/cruzbit/client
$ go install -v github.com/cruzbit/cruzbit/client
```

The cruzbit bins should now be available in your go-managed `$GOPATH/bin` (which is hopefully also on your `$PATH`). You can test this by running `client -h` to print the help screen.

## CLI Options

Client help is available via the `client -h` command:

```
$ client -h
Usage of /home/cruzbit/go/bin/client:
  -compress
        Compress blocks on disk with lz4
  -datadir string
        Path to a directory to save block chain data
  -dnsseed
        Run a DNS server to allow others to find peers
  -inlimit int
        Limit for the number of inbound peer connections. (default 128)
  -keyfile string
        Path to a file containing public keys to use when mining
  -memo string
        A memo to include in newly mined blocks
  -noaccept
        Disable inbound peer connections
  -noirc
        Disable use of IRC for peer discovery
  -numminers int
        Number of miners to run (default 1)
  -peer string
        Address of a peer to connect to
  -port int
        Port to listen for incoming peer connections (default 8831)
  -prune
        Prune transaction and public key transaction indices
  -pubkey string
        A public key which receives newly mined block rewards
  -tlscert string
        Path to a file containing a PEM-encoded X.509 certificate to use with TLS
  -tlskey string
        Path to a file containing a PEM-encoded private key to use with TLS
  -upnp
        Attempt to forward the cruzbit port on your router with UPnP
```

## Running the Client

The client requires a data dir for storage of the cruzbit blockchain and general metadata as well as one or more public keys to send block rewards to upon mining. Otherwise, running the client is as simple as:

```
$ client -datadir cruzbit-chain -keyfile keys.txt -numminers 2
```

### Configuring Peer Discovery

The client supports two modes of peer discovery: DNS with IRC as fallback.

If you want to run a DNS server to help enable peer discovery, you can pass the `-dnsseed` flag.

If you wish to disable IRC discovery, that can be disabled via the `-noirc` flag.

If you wish to enable UPnP port forwarding for the client node, use the `-upnp` flag.

### Configuring Miners

In order to effectively mine, you'll typically want to run one miner per CPU core on your machine. This is configured via the `-numminers` param, like so:

```
$ client ... -numminers 4
```

To run a miner-less node, you can pass `0` as the number of miners like so:

```
$ client ... -numminers 0
```

### Configuring Keys

The client supports two modes of block reward transactions for mining: single key and key list targets.

To distribute block rewards to a single key, use the `-pubkey` flag to pass the target public key in the CLI command.

To distribute block rewards to multiple keys, use the `-keyfile` flag with a text file of the public keys (one per line).

> NOTE: The wallet components `dumpkeys` command will generate a `keys.txt` for you as part of wallet setup.

## Terminating the client

The client runs synchronously in the current window, so to exit simply hit control-c for a graceful shutdown.
