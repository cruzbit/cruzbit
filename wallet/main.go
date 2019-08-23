// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/c-bata/go-prompt"
	. "github.com/cruzbit/cruzbit"
	"github.com/logrusorgru/aurora"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh/terminal"
)

// This is a lightweight wallet client. It pretty much does the bare minimum at the moment so we can test the system
func main() {
	rand.Seed(time.Now().UnixNano())

	DefaultPeer := "127.0.0.1:" + strconv.Itoa(DEFAULT_CRUZBIT_PORT)
	peerPtr := flag.String("peer", DefaultPeer, "Address of a peer to connect to")
	dbPathPtr := flag.String("walletdb", "", "Path to a wallet database (created if it doesn't exist)")
	tlsVerifyPtr := flag.Bool("tlsverify", false, "Verify the TLS certificate of the peer is signed by a recognized CA and the host matches the CN")
	recoverPtr := flag.Bool("recover", false, "Attempt to recover a corrupt walletdb")
	flag.Parse()

	if len(*dbPathPtr) == 0 {
		log.Fatal("Path to the wallet database required")
	}
	if len(*peerPtr) == 0 {
		log.Fatal("Peer address required")
	}
	// add default port, if one was not supplied
	i := strings.LastIndex(*peerPtr, ":")
	if i < 0 {
		*peerPtr = *peerPtr + ":" + strconv.Itoa(DEFAULT_CRUZBIT_PORT)
	}

	// load genesis block
	var genesisBlock Block
	if err := json.Unmarshal([]byte(GenesisBlockJson), &genesisBlock); err != nil {
		log.Fatal(err)
	}
	genesisID, err := genesisBlock.ID()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Starting up...")
	fmt.Printf("Genesis block ID: %s\n", genesisID)

	if *recoverPtr {
		fmt.Println("Attempting to recover wallet...")
	}

	// instantiate wallet
	wallet, err := NewWallet(*dbPathPtr, *recoverPtr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		// load wallet passphrase
		passphrase := promptForPassphrase()
		ok, err := wallet.SetPassphrase(passphrase)
		if err != nil {
			log.Fatal(err)
		}
		if ok {
			break
		}
		fmt.Println(aurora.Bold(aurora.Red("Passphrase is not the one used to encrypt your most recent key.")))
	}

	// connect the wallet ondemand
	connectWallet := func() error {
		if wallet.IsConnected() {
			return nil
		}
		if err := wallet.Connect(*peerPtr, genesisID, *tlsVerifyPtr); err != nil {
			return err
		}
		go wallet.Run()
		return wallet.SetFilter()
	}

	var newTxs []*Transaction
	var newConfs []*transactionWithHeight
	var newTxsLock, newConfsLock, cmdLock sync.Mutex

	// handle new incoming transactions
	wallet.SetTransactionCallback(func(tx *Transaction) {
		ok, err := transactionIsRelevant(wallet, tx)
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			return
		}
		if !ok {
			// false positive
			return
		}
		newTxsLock.Lock()
		showMessage := len(newTxs) == 0
		newTxs = append(newTxs, tx)
		newTxsLock.Unlock()
		if showMessage {
			go func() {
				// don't interrupt a user during a command
				cmdLock.Lock()
				defer cmdLock.Unlock()
				fmt.Printf("\n\nNew incoming transaction! ")
				fmt.Printf("Type %s to view it.\n\n",
					aurora.Bold(aurora.Green("show")))
			}()
		}
	})

	// handle new incoming filter blocks
	wallet.SetFilterBlockCallback(func(fb *FilterBlockMessage) {
		for _, tx := range fb.Transactions {
			ok, err := transactionIsRelevant(wallet, tx)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				continue
			}
			if !ok {
				// false positive
				continue
			}
			newConfsLock.Lock()
			showMessage := len(newConfs) == 0
			newConfs = append(newConfs, &transactionWithHeight{tx: tx, height: fb.Header.Height})
			newConfsLock.Unlock()
			if showMessage {
				go func() {
					// don't interrupt a user during a command
					cmdLock.Lock()
					defer cmdLock.Unlock()
					fmt.Printf("\n\nNew transaction confirmation! ")
					fmt.Printf("Type %s to view it.\n\n",
						aurora.Bold(aurora.Green("conf")))
				}()
			}
		}
	})

	// setup prompt
	completer := func(d prompt.Document) []prompt.Suggest {
		s := []prompt.Suggest{
			{Text: "newkey", Description: "Generate and store a new private key"},
			{Text: "listkeys", Description: "List all known public keys"},
			{Text: "genkeys", Description: "Generate multiple keys at once"},
			{Text: "dumpkeys", Description: "Dump all of the wallet's public keys to a text file"},
			{Text: "balance", Description: "Retrieve the current balance of all public keys"},
			{Text: "send", Description: "Send cruzbits to someone"},
			{Text: "show", Description: "Show new incoming transactions"},
			{Text: "txstatus", Description: "Show confirmed transaction information given a transaction ID"},
			{Text: "clearnew", Description: "Clear all pending incoming transaction notifications"},
			{Text: "conf", Description: "Show new transaction confirmations"},
			{Text: "clearconf", Description: "Clear all pending transaction confirmation notifications"},
			{Text: "rewards", Description: "Show immature block rewards for all public keys"},
			{Text: "verify", Description: "Verify the private key is decryptable and intact for all public keys displayed with 'listkeys'"},
			{Text: "export", Description: "Save all of the wallet's public-private key pairs to a text file"},
			{Text: "quit", Description: "Quit this wallet session"},
		}
		return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), true)
	}

	fmt.Println("Please select a command.")
	fmt.Printf("To connect to your wallet peer you need to issue a command requiring it, e.g. %s\n",
		aurora.Bold(aurora.Green("balance")))
	for {
		// run interactive prompt
		cmd := prompt.Input("> ", completer)
		cmdLock.Lock()
		switch cmd {
		case "newkey":
			pubKeys, err := wallet.NewKeys(1)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			fmt.Printf("New key generated, public key: %s\n",
				aurora.Bold(base64.StdEncoding.EncodeToString(pubKeys[0][:])))
			if wallet.IsConnected() {
				// update our filter if online
				if err := wallet.SetFilter(); err != nil {
					fmt.Printf("Error: %s\n", err)
				}
			}

		case "listkeys":
			pubKeys, err := wallet.GetKeys()
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			for i, pubKey := range pubKeys {
				fmt.Printf("%4d: %s\n",
					i+1, base64.StdEncoding.EncodeToString(pubKey[:]))
			}

		case "genkeys":
			count, err := promptForNumber("Count", 4, bufio.NewReader(os.Stdin))
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			if count <= 0 {
				break
			}
			pubKeys, err := wallet.NewKeys(count)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			fmt.Printf("Generated %d new keys\n", len(pubKeys))
			if wallet.IsConnected() {
				// update our filter if online
				if err := wallet.SetFilter(); err != nil {
					fmt.Printf("Error: %s\n", err)
				}
			}

		case "dumpkeys":
			pubKeys, err := wallet.GetKeys()
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			if len(pubKeys) == 0 {
				fmt.Printf("No public keys found\n")
				break
			}
			name := "keys.txt"
			f, err := os.Create(name)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			for _, pubKey := range pubKeys {
				key := fmt.Sprintf("%s\n", base64.StdEncoding.EncodeToString(pubKey[:]))
				f.WriteString(key)
			}
			f.Close()
			fmt.Printf("%d public keys saved to '%s'\n", len(pubKeys), aurora.Bold(name))

		case "balance":
			if err := connectWallet(); err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			pubKeys, err := wallet.GetKeys()
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			var total int64
			for i, pubKey := range pubKeys {
				balance, _, err := wallet.GetBalance(pubKey)
				if err != nil {
					fmt.Printf("Error: %s\n", err)
					break
				}
				amount := roundFloat(float64(balance), 8) / CRUZBITS_PER_CRUZ
				fmt.Printf("%4d: %s %16.8f\n",
					i+1,
					base64.StdEncoding.EncodeToString(pubKey[:]),
					amount)
				total += balance
			}
			amount := roundFloat(float64(total), 8) / CRUZBITS_PER_CRUZ
			fmt.Printf("%s: %.8f\n", aurora.Bold("Total"), amount)

		case "send":
			if err := connectWallet(); err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			id, err := sendTransaction(wallet)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			fmt.Printf("Transaction %s sent\n", id)

		case "txstatus":
			if err := connectWallet(); err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			txID, err := promptForTransactionID("ID", 2, bufio.NewReader(os.Stdin))
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			fmt.Println("")
			tx, _, height, err := wallet.GetTransaction(txID)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			if tx == nil {
				fmt.Printf("Transaction %s not found in the blockchain at this time.\n",
					txID)
				fmt.Println("It may be waiting for confirmation.")
				break
			}
			showTransaction(wallet, tx, height)

		case "show":
			if err := connectWallet(); err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			tx, left := func() (*Transaction, int) {
				newTxsLock.Lock()
				defer newTxsLock.Unlock()
				if len(newTxs) == 0 {
					return nil, 0
				}
				tx := newTxs[0]
				newTxs = newTxs[1:]
				return tx, len(newTxs)
			}()
			if tx != nil {
				showTransaction(wallet, tx, 0)
				if left > 0 {
					fmt.Printf("\n%d new transaction(s) left to display. Type %s to continue.\n",
						left, aurora.Bold(aurora.Green("show")))
				}
			} else {
				fmt.Printf("No new transactions to display\n")
			}

		case "clearnew":
			func() {
				newTxsLock.Lock()
				defer newTxsLock.Unlock()
				newTxs = nil
			}()

		case "conf":
			if err := connectWallet(); err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			tx, left := func() (*transactionWithHeight, int) {
				newConfsLock.Lock()
				defer newConfsLock.Unlock()
				if len(newConfs) == 0 {
					return nil, 0
				}
				tx := newConfs[0]
				newConfs = newConfs[1:]
				return tx, len(newConfs)
			}()
			if tx != nil {
				showTransaction(wallet, tx.tx, tx.height)
				if left > 0 {
					fmt.Printf("\n%d new confirmations(s) left to display. Type %s to continue.\n",
						left, aurora.Bold(aurora.Green("conf")))
				}
			} else {
				fmt.Printf("No new confirmations to display\n")
			}

		case "clearconf":
			func() {
				newConfsLock.Lock()
				defer newConfsLock.Unlock()
				newConfs = nil
			}()

		case "rewards":
			if err := connectWallet(); err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			pubKeys, err := wallet.GetKeys()
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			_, tipHeader, err := wallet.GetTipHeader()
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			var total int64
			lastHeight := tipHeader.Height - COINBASE_MATURITY
		gpkt:
			for i, pubKey := range pubKeys {
				var rewards, startHeight int64 = 0, lastHeight + 1
				var startIndex int = 0
				for {
					_, stopHeight, stopIndex, fbs, err := wallet.GetPublicKeyTransactions(
						pubKey, startHeight, tipHeader.Height+1, startIndex, 32)
					if err != nil {
						fmt.Printf("Error: %s\n", err)
						break gpkt
					}
					var numTx int
					startHeight, startIndex = stopHeight, stopIndex+1
					for _, fb := range fbs {
						for _, tx := range fb.Transactions {
							numTx++
							if tx.IsCoinbase() {
								rewards += tx.Amount
							}
						}
					}
					if numTx < 32 {
						break
					}
				}
				amount := roundFloat(float64(rewards), 8) / CRUZBITS_PER_CRUZ
				fmt.Printf("%4d: %s %16.8f\n",
					i+1,
					base64.StdEncoding.EncodeToString(pubKey[:]),
					amount)
				total += rewards
			}
			amount := roundFloat(float64(total), 8) / CRUZBITS_PER_CRUZ
			fmt.Printf("%s: %.8f\n", aurora.Bold("Total"), amount)

		case "verify":
			pubKeys, err := wallet.GetKeys()
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			var verified, corrupt int
			for i, pubKey := range pubKeys {
				if err := wallet.VerifyKey(pubKey); err != nil {
					corrupt++
					fmt.Printf("%4d: %s %s\n",
						i+1, base64.StdEncoding.EncodeToString(pubKey[:]),
						aurora.Bold(aurora.Red(err.Error())))
				} else {
					verified++
					fmt.Printf("%4d: %s %s\n",
						i+1, base64.StdEncoding.EncodeToString(pubKey[:]),
						aurora.Bold(aurora.Green("Verified")))
				}
			}
			fmt.Printf("%d key(s) verified and %d key(s) potentially corrupt\n",
				verified, corrupt)

		case "export":
			fmt.Println(aurora.BrightRed("WARNING"), aurora.Bold(": Anyone with access to a wallet's " +
				"private key(s) has full control of the funds in the wallet."))
			confirm, err := promptForConfirmation("Are you sure you wish to proceed?", false,
				bufio.NewReader(os.Stdin))
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			if !confirm {
				fmt.Println("Aborting export")
				break
			}
			pubKeys, err := wallet.GetKeys()
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			if len(pubKeys) == 0 {
				fmt.Printf("No private keys found\n")
				break
			}
			name := "export.txt"
			f, err := os.Create(name)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				break
			}
			count := 0
			for _, pubKey := range pubKeys {
				private, err := wallet.GetPrivateKey(pubKey)
				if err != nil {
					fmt.Printf("Couldn't get private key for public key: %s; omitting from export\n", pubKey)
					continue
				}
				pair := fmt.Sprintf("%s,%s\n",
					base64.StdEncoding.EncodeToString(pubKey[:]),
					base64.StdEncoding.EncodeToString(private[:]))
				f.WriteString(pair)
				count++
			}
			f.Close()
			fmt.Printf("%d wallet key pairs saved to '%s'\n", count, aurora.Bold(name))

		case "quit":
			wallet.Shutdown()
			return
		}

		fmt.Println("")
		cmdLock.Unlock()
	}
}

// Prompt for transaction details and request the wallet to send it
func sendTransaction(wallet *Wallet) (TransactionID, error) {
	minFee, minAmount, err := wallet.GetTransactionRelayPolicy()
	if err != nil {
		return TransactionID{}, err
	}

	reader := bufio.NewReader(os.Stdin)

	// prompt for from
	from, err := promptForPublicKey("From", 6, reader)
	if err != nil {
		return TransactionID{}, err
	}

	// prompt for to
	to, err := promptForPublicKey("To", 6, reader)
	if err != nil {
		return TransactionID{}, err
	}

	// prompt for amount
	amount, err := promptForValue("Amount", 6, reader)
	if err != nil {
		return TransactionID{}, err
	}
	if amount < minAmount {
		return TransactionID{}, fmt.Errorf(
			"The peer's minimum amount to relay transactions is %.8f",
			roundFloat(float64(minAmount), 8)/CRUZBITS_PER_CRUZ)
	}

	// prompt for fee
	fee, err := promptForValue("Fee", 6, reader)
	if err != nil {
		return TransactionID{}, err
	}
	if fee < minFee {
		return TransactionID{}, fmt.Errorf(
			"The peer's minimum required fee to relay transactions is %.8f",
			roundFloat(float64(minFee), 8)/CRUZBITS_PER_CRUZ)
	}

	// prompt for memo
	fmt.Printf("%6v: ", aurora.Bold("Memo"))
	text, err := reader.ReadString('\n')
	if err != nil {
		return TransactionID{}, err
	}
	memo := strings.TrimSpace(text)
	if len(memo) > MAX_MEMO_LENGTH {
		return TransactionID{}, fmt.Errorf("Maximum memo length (%d) exceeded (%d)",
			MAX_MEMO_LENGTH, len(memo))
	}

	// create and send send it. by default the transaction expires if not mined within 3 blocks from now
	id, err := wallet.Send(from, to, amount, fee, 0, 3, memo)
	if err != nil {
		return TransactionID{}, err
	}
	return id, nil
}

func promptForPublicKey(prompt string, rightJustify int, reader *bufio.Reader) (ed25519.PublicKey, error) {
	fmt.Printf("%"+strconv.Itoa(rightJustify)+"v: ", aurora.Bold(prompt))
	text, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	pubKeyBytes, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		return nil, err
	}
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("Invalid public key")
	}
	return ed25519.PublicKey(pubKeyBytes), nil
}

func promptForValue(prompt string, rightJustify int, reader *bufio.Reader) (int64, error) {
	fmt.Printf("%"+strconv.Itoa(rightJustify)+"v: ", aurora.Bold(prompt))
	text, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	text = strings.TrimSpace(text)
	if len(text) == 0 {
		return 0, nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("Invalid value")
	}
	valueInt := int64(roundFloat(value, 8) * CRUZBITS_PER_CRUZ)
	return valueInt, nil
}

func promptForNumber(prompt string, rightJustify int, reader *bufio.Reader) (int, error) {
	fmt.Printf("%"+strconv.Itoa(rightJustify)+"v: ", aurora.Bold(prompt))
	text, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(text))
}

func promptForConfirmation(prompt string, defaultResponse bool, reader *bufio.Reader) (bool, error) {
	defaultPrompt := " [y/N]"
	if defaultResponse {
		defaultPrompt = " [Y/n]"
	}
	fmt.Printf("%v:", aurora.Bold(prompt+defaultPrompt))
	text, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	text = strings.ToLower(strings.TrimSpace(text))
	switch text {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
	}
	return defaultResponse, nil
}

func promptForTransactionID(prompt string, rightJustify int, reader *bufio.Reader) (TransactionID, error) {
	fmt.Printf("%"+strconv.Itoa(rightJustify)+"v: ", aurora.Bold(prompt))
	text, err := reader.ReadString('\n')
	if err != nil {
		return TransactionID{}, err
	}
	text = strings.TrimSpace(text)
	if len(text) != 2*(len(TransactionID{})) {
		return TransactionID{}, fmt.Errorf("Invalid transaction ID")
	}
	idBytes, err := hex.DecodeString(text)
	if err != nil {
		return TransactionID{}, err
	}
	if len(idBytes) != len(TransactionID{}) {
		return TransactionID{}, fmt.Errorf("Invalid transaction ID")
	}
	var id TransactionID
	copy(id[:], idBytes)
	return id, nil
}

func showTransaction(w *Wallet, tx *Transaction, height int64) {
	when := time.Unix(tx.Time, 0)
	id, _ := tx.ID()
	fmt.Printf("%7v: %s\n", aurora.Bold("ID"), id)
	fmt.Printf("%7v: %d\n", aurora.Bold("Series"), tx.Series)
	fmt.Printf("%7v: %s\n", aurora.Bold("Time"), when)
	if tx.From != nil {
		fmt.Printf("%7v: %s\n", aurora.Bold("From"), base64.StdEncoding.EncodeToString(tx.From))
	}
	fmt.Printf("%7v: %s\n", aurora.Bold("To"), base64.StdEncoding.EncodeToString(tx.To))
	fmt.Printf("%7v: %.8f\n", aurora.Bold("Amount"), roundFloat(float64(tx.Amount), 8)/CRUZBITS_PER_CRUZ)
	if tx.Fee > 0 {
		fmt.Printf("%7v: %.8f\n", aurora.Bold("Fee"), roundFloat(float64(tx.Fee), 8)/CRUZBITS_PER_CRUZ)
	}
	if len(tx.Memo) > 0 {
		fmt.Printf("%7v: %s\n", aurora.Bold("Memo"), tx.Memo)
	}

	_, header, _ := w.GetTipHeader()
	if height <= 0 {
		if tx.Matures > 0 {
			fmt.Printf("%7v: cannot be mined until height: %d, current height: %d\n",
				aurora.Bold("Matures"), tx.Matures, header.Height)
		}
		if tx.Expires > 0 {
			fmt.Printf("%7v: cannot be mined after height: %d, current height: %d\n",
				aurora.Bold("Expires"), tx.Expires, header.Height)
		}
		return
	}

	fmt.Printf("%7v: confirmed at height %d, %d confirmation(s)\n",
		aurora.Bold("Status"), height, (header.Height-height)+1)
}

// Catch filter false-positives
func transactionIsRelevant(wallet *Wallet, tx *Transaction) (bool, error) {
	pubKeys, err := wallet.GetKeys()
	if err != nil {
		return false, err
	}
	for _, pubKey := range pubKeys {
		if tx.Contains(pubKey) {
			return true, nil
		}
	}
	return false, nil
}

// secure passphrase prompt helper
func promptForPassphrase() string {
	var passphrase string
	for {
		q := "Enter"
		if len(passphrase) != 0 {
			q = "Confirm"
		}
		fmt.Printf("\n%s passphrase: ", q)
		ppBytes, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatal(err)
		}
		if len(passphrase) != 0 {
			if passphrase != string(ppBytes) {
				passphrase = ""
				fmt.Printf("\nPassphrase mismatch\n")
				continue
			}
			break
		}
		passphrase = string(ppBytes)
	}
	fmt.Printf("\n\n")
	return passphrase
}

type transactionWithHeight struct {
	tx     *Transaction
	height int64
}

// From: https://groups.google.com/forum/#!topic/golang-nuts/ITZV08gAugI
func roundFloat(x float64, prec int) float64 {
	var rounder float64
	pow := math.Pow(10, float64(prec))
	intermed := x * pow
	_, frac := math.Modf(intermed)
	intermed += .5
	x = .5
	if frac < 0.0 {
		x = -.5
		intermed -= 1
	}
	if frac >= x {
		rounder = math.Ceil(intermed)
	} else {
		rounder = math.Floor(intermed)
	}

	return rounder / pow
}
