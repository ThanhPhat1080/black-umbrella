// Package e2e implements the Umbrella message envelope: a random content key
// encrypts the body with secretbox and is wrapped for every recipient with box.
package e2e

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

var ErrDecrypt = errors.New("decrypt failed")

type KeyPair struct{ Public, Private [32]byte }

type Sealed struct {
	Nonce, CT string
	Keys      map[string]string
}

func GenerateKeyPair() (KeyPair, error) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return KeyPair{}, err
	}
	return KeyPair{Public: *pub, Private: *priv}, nil
}

type payload struct {
	Text string `json:"text"`
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := io.ReadFull(rand.Reader, b)
	return b, err
}

// Seal encrypts text for every recipient (the sender should be included in recipients).
func Seal(text string, sender KeyPair, recipients map[string][32]byte) (Sealed, error) {
	body, err := json.Marshal(payload{Text: text})
	if err != nil {
		return Sealed{}, err
	}
	kb, err := randomBytes(32)
	if err != nil {
		return Sealed{}, err
	}
	var k [32]byte
	copy(k[:], kb)
	nb, err := randomBytes(24)
	if err != nil {
		return Sealed{}, err
	}
	var nonce [24]byte
	copy(nonce[:], nb)
	out := Sealed{
		Nonce: base64.StdEncoding.EncodeToString(nonce[:]),
		CT:    base64.StdEncoding.EncodeToString(secretbox.Seal(nil, body, &nonce, &k)),
		Keys:  make(map[string]string, len(recipients)),
	}
	for name, pub := range recipients {
		wb, err := randomBytes(24)
		if err != nil {
			return Sealed{}, err
		}
		var wn [24]byte
		copy(wn[:], wb)
		pub := pub
		out.Keys[name] = base64.StdEncoding.EncodeToString(box.Seal(wn[:], k[:], &wn, &pub, &sender.Private))
	}
	return out, nil
}

// Open decrypts a message addressed to me, authenticating the wrapped key to senderPub.
func Open(nonce, ct, wrappedKey string, me KeyPair, senderPub [32]byte) (string, error) {
	wk, err := base64.StdEncoding.DecodeString(wrappedKey)
	if err != nil || len(wk) < 24 {
		return "", ErrDecrypt
	}
	var wn [24]byte
	copy(wn[:], wk[:24])
	kb, ok := box.Open(nil, wk[24:], &wn, &senderPub, &me.Private)
	if !ok || len(kb) != 32 {
		return "", ErrDecrypt
	}
	var k [32]byte
	copy(k[:], kb)
	nb, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil || len(nb) != 24 {
		return "", ErrDecrypt
	}
	var n [24]byte
	copy(n[:], nb)
	cb, err := base64.StdEncoding.DecodeString(ct)
	if err != nil {
		return "", ErrDecrypt
	}
	pt, ok := secretbox.Open(nil, cb, &n, &k)
	if !ok {
		return "", ErrDecrypt
	}
	var p payload
	if err := json.Unmarshal(pt, &p); err != nil {
		return "", ErrDecrypt
	}
	return p.Text, nil
}
