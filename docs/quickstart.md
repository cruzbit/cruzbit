# Quickstart

## Overview

Using cruzbit requires two components: a client and a wallet.

### Wallet

The wallet component is the user facing component for coin management. It's responsible for private key management and user-driven network transactions (such as viewing balance or sending/receiving cruzbit).

### Client

The client is the component responsible for maintaining a peering connection to the cruzbit network (i.e. running a cruzbit node) and mining. The client uses a peer discovery protocol to bootstrap itself onto the cruzbit network and then cooperates with other nodes to manage the distributed ledger.

Miners running in the client are responsible for mining new blocks of cruzbit in coordination with the cruzbit network. When a miner running on your local cruzbit node mines a new block it will automatically create a transaction on the network sending the block reward to one of your wallet-managed public keys. Currently, each block mined nets you a reward of 50 cruz (or 5,000,000,000 cruzbits).

## Pre-requisites

To build and install cruzbit, you'll need the [Go language](https://golang.org/doc/install) runtime and compilation tools. You can get that by installing [Go](https://golang.org/doc/install#install) using the latest installation guide:

- https://golang.org/doc/install#install

Or using the [Cruzbit for Linux Quickstart](https://gist.github.com/setanimals/f562ed7dd1c69af3fbe960c7b9502615).

## Installation

To get started, let's build and install both the `client` and `wallet` components:

```
$ export GO111MODULE=on
$ go get -v github.com/cruzbit/cruzbit/client github.com/cruzbit/cruzbit/wallet
$ go install -v github.com/cruzbit/cruzbit/client github.com/cruzbit/cruzbit/wallet
```

The cruzbit bins should now be available in your Go-managed `$GOPATH/bin` (which is hopefully also on your `$PATH`). You can test this by running e.g. `client -h` or `$GOPATH/bin/client -h` to print the CLI help screen.

## Wallet Setup

First, we'll need to initialize the wallet database and setup a wallet passphrase that will be used to encrypt the private keys. The wallet will need a secure dir that should be backed up (after generating any new keys) to avoid loss of private keys. Be sure to quit the wallet session before conducting any backups. Start up the wallet like so:

```
$ wallet -walletdb cruzbit-wallet
Starting up...
Genesis block ID: 00000000e29a7850088d660489b7b9ae2da763bc3bd83324ecc54eee04840adb

Enter passphrase: <enter new passphrase here>
Confirm passphrase: <enter new passphrase here>

Please select a command.
To connect to your wallet peer you need to issue a command requiring it, e.g. balance
>
```

!> Note: Once set, the passphrase will now be required to decrypt the walletdb in future runs - so make sure to remember it.

### Key Pair Generation

Generate one or more key pairs using the `genkeys` command:

```
Please select a command.
To connect to your wallet peer you need to issue a command requiring it, e.g. balance
> genkeys
Count: 2
Generated 2 new keys
```

These keys will later be used to send and receive transactions on the network from miner instances or other wallets.

### Create a Key File

Create a plaintext list of the newly generated public keys (in a `keys.txt` file) by using the `dumpkeys` command:

```
> dumpkeys
2 public keys saved to 'keys.txt'
```

## Running the Client

Given the newly created keyfile, we're ready to connect to run the client and begin mining:

```
$ client -datadir cruzbit-node -keyfile keys.txt -numminers 4 -upnp
```

!> Note: To enable constant mining, make sure the `client` process stays running in either `screen` or another durable session.

## Check Your Balance

Once the client has spun up, you should now be able to issue the `balance` command in your wallet to check your current balance:

```
> balance
   1: GVoqW1OmLD5QpnthuU5w4ZPNd6Me8NFTQLxfBsFNJVo=        0.00000000
   2: Y1ob+lgssGw7hDjhUvkM1XwAUr00EYQrAN2W3Z13T/g=       50.00000000
Total: 50.00000000
```

Like bitcoin, any blocks you mine will need to have an additional 100 blocks mined on top of them prior to the new cruzbits being applied to your balance. This is to mitigate a potentially poor user experience in the case of honest blockchain reorganizations.

The wallet will also watch for and notify you about new transaction confirmations to any of your configured public key addresses.

See the [Wallet](wallet.md) and [Client](client.md) help pages for more information on the CLI options.