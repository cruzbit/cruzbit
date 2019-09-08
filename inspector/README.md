![cruzbit_logo_v1 half](https://user-images.githubusercontent.com/51346587/64493652-8ea93980-d237-11e9-8bee-681494eb365b.png)

# inspector

The inspector is a simple tool for examining the offline block chain data

## To install

1. Make sure you have the new Go modules support enabled: `export GO111MODULE=on`
2. `go install github.com/cruzbit/cruzbit/inspector`

The `inspector` application is now in `$HOME/go/bin/inspector`.

## Basic command line arguments

`inspector -datadir <block chain data directory> -command <command> [other flags required per command]`

## Commands

* **height** - Display the current block chain height.
* **balance** - Display the current balance for the public key specified with `-pubkey`.
* **balance_at** - Display the balance for the public key specified with `-pubkey` for the given height specified with `-height`.
* **block** - Display the block specified with `-block_id`.
* **block_at** - Display the block at the block chain height specified with `-height`.
* **tx** - Display the transaction specified with `-tx_id`.
* **history** - Display transaction history for the public key specified with `-pubkey`. Other options for this command include `-start_height`, `-end_height`, `-start_index`, and `-limit`.
* **verify** - Verify the sum of all public key balances matches what's expected dictated by the block reward schedule. If `-pubkey` is specified, it verifies the public key's balance matches the balance computed using the public key's transaction history.
