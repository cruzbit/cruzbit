// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"golang.org/x/crypto/ed25519"
)

func TestTransaction(t *testing.T) {
	// create a sender
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	// create a recipient
	pubKey2, privKey2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}

	// create the unsigned transaction
	tx := NewTransaction(pubKey, pubKey2, 50*CRUZBITS_PER_CRUZ, 0, 0, 0, 0, "for lunch")

	// sign the transaction
	if err := tx.Sign(privKey); err != nil {
		t.Fatal(err)
	}

	// verify the transaction
	ok, err := tx.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("Verification failed")
	}

	// re-sign the transaction with the wrong private key
	if err := tx.Sign(privKey2); err != nil {
		t.Fatal(err)
	}

	// verify the transaction (should fail)
	ok, err = tx.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("Expected verification failure")
	}
}

func TestTransactionTestVector1(t *testing.T) {
	// create transaction for Test Vector 1
	pubKeyBytes, err := base64.StdEncoding.DecodeString("80tvqyCax0UdXB+TPvAQwre7NxUHhISm/bsEOtbF+yI=")
	if err != nil {
		t.Fatal(err)
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	pubKeyBytes2, err := base64.StdEncoding.DecodeString("YkJHRtoQDa1TIKhN7gKCx54bavXouJy4orHwcRntcZY=")
	if err != nil {
		t.Fatal(err)
	}
	pubKey2 := ed25519.PublicKey(pubKeyBytes2)

	tx := NewTransaction(pubKey, pubKey2, 50*CRUZBITS_PER_CRUZ, 2*CRUZBITS_PER_CRUZ, 0, 0, 0, "for lunch")
	tx.Time = 1558565474
	tx.Nonce = 2019727887

	// check JSON matches test vector
	txJson, err := json.Marshal(tx)
	if err != nil {
		t.Fatal(err)
	}
	if string(txJson) != `{"time":1558565474,"nonce":2019727887,"from":"80tvqyCax0UdXB+TPvAQwre7NxUHhISm/bsEOtbF+yI=",`+
		`"to":"YkJHRtoQDa1TIKhN7gKCx54bavXouJy4orHwcRntcZY=","amount":5000000000,"fee":200000000,"memo":"for lunch","series":1}` {
		t.Fatal("JSON differs from test vector: " + string(txJson))
	}

	// check ID matches test vector
	id, err := tx.ID()
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "fc04870db147eb31823ce7c68ef366a7e94c2a719398322d746ddfd0f5c98776" {
		t.Fatalf("ID %s differs from test vector", id)
	}

	// add signature from test vector
	sigBytes, err := base64.StdEncoding.DecodeString("Fgb3q77evL5jZIXHMrpZ+wBOs2HZx07WYehi6EpHSlvnRv4wPvrP2sTTzAAmdvJZlkLrHXw1ensjXBiDosucCw==")
	if err != nil {
		t.Fatal(err)
	}
	tx.Signature = Signature(sigBytes)

	// verify the transaction
	ok, err := tx.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("Verification failed")
	}

	// re-sign the transaction with private key from test vector
	privKeyBytes, err := base64.StdEncoding.DecodeString("EBQtXb3/Ht6KFh8/+Lxk9aDv2Zrag5G8r+dhElbCe07zS2+rIJrHRR1cH5M+8BDCt7s3FQeEhKb9uwQ61sX7Ig==")
	if err != nil {
		t.Fatal(err)
	}
	privKey := ed25519.PrivateKey(privKeyBytes)
	if err := tx.Sign(privKey); err != nil {
		t.Fatal(err)
	}

	// verify the transaction
	ok, err = tx.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("Verification failed")
	}

	// re-sign the transaction with the wrong private key
	_, privKey2, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Sign(privKey2); err != nil {
		t.Fatal(err)
	}

	// verify the transaction (should fail)
	ok, err = tx.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Errorf("Expected verification failure")
	}
}
