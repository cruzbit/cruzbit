# cruzbit
A simple decentralized peer-to-peer ledger implementation

cruzbit is very similar to [bitcoin](https://www.bitcoin.com/bitcoin.pdf) with the following notable differences:

* **Newer crypto** - The [Ed25519 signature system](https://ed25519.cr.yp.to/) is used for signing transactions. This system has a number of nice properties to protect users from security risks present with naive usage of ECDSA. The 256-bit version of the [SHA-3 hashing algorithm](https://en.wikipedia.org/wiki/SHA-3) is used for all non-signing hashing operations, including the proof-of-work function. It's reported to be 
[blazing fast](https://keccak.team/2017/is_sha3_slow.html) when implemented in hardware. [NaCl Secretbox](https://nacl.cr.yp.to/secretbox.html) is used to encrypt wallet private keys (not part of the protocol.)
* **Simplified transaction format** - No inputs and outputs. Just public key sender and receiver with a time, amount, explicit fee, memo field, pseudo-random nonce, series and signature. The series is incremented network-wide roughly once a week based on block height to allow for pruning transaction history. Also included are 2 optional fields for specifying maturity and expiration, both at a given block height.
* **No UTXO set** - This is a consequence of the second point. It considerably simplifies ledger construction and management as well as requires a wallet to know only about its public key balances and the current block height. It also allows the ledger to map more directly to the well-understood concept of a [double-entry bookkeeping system](https://en.wikipedia.org/wiki/Double-entry_bookkeeping_system). In cruzbit, the sum of all public key balances must equal the issuance at the current block height. This isn't the first ledger to get rid of the UTXO set model but I think we do it in a uniquely simple way.
* **No scripting** - This is another consequence of the second point. Signatures are simply signatures and not tiny scripts. It's a bit simpler and arguably safer. It does limit functionality, e.g. there is no native notion of a multi-signature transaction, however, depending on your needs, you can come _close_ to accomplishing that using [mechanisms external to cruzbit](https://en.wikipedia.org/wiki/Shamir%27s_Secret_Sharing).
* **No fixed block size limit** - Since transactions in cruzbit are more-or-less fixed size we cap blocks by transaction count instead, with the initial limit being 10,000 transactions. This per-block transaction limit increases with "piecewise-linear-between-doublings growth." This means the limit doubles roughly every 2 years by block height and increases linearly between doublings up until a hard limit of 2,147,483,647. This was directly inspired by [BIP 101](https://github.com/bitcoin/bips/blob/master/bip-0101.mediawiki). We use block height instead of time since another change in cruzbit is that all block headers contain the height (as well as the total cumulative chain work.)
* **Reference implementation is in [Go](https://golang.org/)** - Perhaps more accessible than C++. Hopefully it makes blockchain programming a bit easier to understand and attracts a wider variety of developer interest.
* **Web-friendly peer protocol** - Peer communication is via secure [WebSockets](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API). And the peer protocol and all primitives are structured in [JSON](https://www.json.org/). This should make working with the protocol easy for just about every modern development environment.

## Why does cruzbit exist?

I noticed most people focusing on making more complex ledgers capable of executing "smart" contracts and/or crypto-magically obscuring transaction details and such. And I think those projects are pretty cool, but I'd always wanted to attempt to do the opposite and implement the simplest decentralized ledger I possibly could given lessons learned from bitcoin. I _think_ that's what cruzbit is. Anything that I thought wasn't strictly necessary in bitcoin, or was otherwise weird, I got rid of. I wanted the design to be conceptually simple and extremely developer-friendly. I finally had some personal time on my hands so I decided, why not. And now cruzbit exists.

## Getting started mining

If you missed out on the opportunity to mine other cryptocurrencies you could give cruzbit a try!

1. [Install Go](https://golang.org/doc/install)
2. Install the [wallet](https://github.com/cruzbit/cruzbit/tree/master/wallet)
3. Run the wallet and issue a `newkey` command. Record the public key.
4. Install the [client](https://github.com/cruzbit/cruzbit/tree/master/client)
5. Run the client using the public key from step 4. as the `-pubkey` argument.

Instead of mining with a single public key, you can also use the wallet to generate many keys and dump the public keys to a text file which the client will accept as a `-keyfile` argument. The wallet commands to do this are `genkeys` and `dumpkeys`.

## Discussion

There is a subreddit for discussing issues and answering any questions you may have. Please feel free to ask anything in [r/cruzbit](https://www.reddit.com/r/cruzbit/) on Reddit.
