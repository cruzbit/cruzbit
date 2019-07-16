# Wallet

## Building the Wallet

To build the latest `wallet` binaries from master, simply invoke the Go toolchain like so:

```
$ export GO111MODULE=on
$ go get -v github.com/cruzbit/cruzbit/wallet
$ go install -v github.com/cruzbit/cruzbit/wallet
```

The cruzbit bins should now be available in your go-managed `$GOPATH/bin` (which is hopefully also on your `$PATH`). You can test this by running e.g. `wallet -h` to print the help screen.

## CLI Options

Client help is available via the `client -h` command:

```
$ wallet -h
Usage of /home/cruz/go/bin/wallet:
  -peer string
        Address of a peer to connect to (default "127.0.0.1:8831")
  -recover
        Attempt to recover a corrupt walletdb
  -tlsverify
        Verify the TLS certificate of the peer is signed by a recognized CA and the host matches the CN
  -walletdb string
        Path to a wallet database (created if it doesn't exist)
```

## Running the Wallet

The `wallet` needs a secure and private data directory to store it's wallet database. This content should be kept in a secure, reliable location and backed up.

To initialize a new wallet database, pass the `-walletdb` flag to the dir you wish to use for a wallet database:

```
$ wallet -walletdb cruzbit-wallet
```

> NOTE: The walletdb directory will be created for you if it it does not exist.

Once the wallet is launched, you'll be prompted for an encryption passphrase which will be set the first time you use the walletdb.

## Wallet Operations

The `wallet` is an interactive tool, so once the database is initialized and you've entered the correct passphrase you'll have the option of performing one of many interactive commands inside the wallet:

Command    | Action
---------- | ------
balance    | Retrieve the current balance of all public keys
clearconf  | Clear all pending transaction confirmation notifications
clearnew   | Clear all pending incoming transaction notifications
conf       | Show new transaction confirmations
dumpkeys   | Dump all of the wallet's public keys to a text file
genkeys    | Generate multiple keys at once
listkeys   | List all known public keys
newkey     | Generate and store a new private key
quit       | Quit this wallet session
rewards    | Show immature block rewards for all public keys
send       | Send cruzbits to someone
show       | Show new incoming transactions
txstatus   | Show confirmed transaction information given a transaction ID
verify     | Verify the private key is decryptable and intact for all public keys displayed with 'listkeys'

### Initializing a Wallet

When you run the wallet for a new walletdb, you'll be prompted to enter a new encryption passphrase. This passphrase will be required every subsequent run to unlock the wallet.

#### Generating Keys

Once the walletdb is initialized, you'll want to generate keys to send and receive transactions on the cruzbit network. This can be achieved with the `genkeys` command and entering the count of keys to generate (1 or more):

```
Please select a command.
To connect to your wallet peer you need to issue a command requiring it, e.g. balance
> genkeys
          genkeys  Generate multiple keys at once  
Count: 2
Generated 2 new keys
```

#### Checking Key Balance

This will generate one or more keys which you should then be able to see with the `balance` command:

```
> balance
   1: GVoqW1OmLD5QpnthuU5w4ZPNd6Me8NFTQLxfBsFNJVo=       0.00000000
   2: Y1ob+lgssGw7hDjhUvkM1XwAUr00EYQrAN2W3Z13T/g=       0.00000000
Total: 0.00000000
```

#### Dumping Key Files

Once the keys are generated, you can use the `dumpkeys` command to create a `keys.txt` to pass to the client's `-keyfile` parameter:

```
> dumpkeys
2 public keys saved to 'keys.txt'
> quit

$ cat keys.txt 
GVoqW1OmLD5QpnthuU5w4ZPNd6Me8NFTQLxfBsFNJVo=
Y1ob+lgssGw7hDjhUvkM1XwAUr00EYQrAN2W3Z13T/g=
```

## Troubleshooting

### Connection Issues

Sometimes, the wallet won't be able to connect to a local peer to perform operations like `balance` with an error message like so:

```
Please select a command.
To connect to your wallet peer you need to issue a command requiring it, e.g. balance

> balance
Error: dial tcp 127.0.0.1:8831: connect: connection refused
```

To resolve this, please ensure the `client` component is running and connected to the network. There is a slight startup delay for the `client` process to be available to the `wallet` after starting.
