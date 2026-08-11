// Package secretbox is an authenticated-encryption adapter for small secrets
// held at rest -- specifically a BYOC cluster's self-contained kubeconfig,
// which Honryu stores encrypted in MySQL and decrypts only to materialize a
// home-cluster k8s Secret. It wraps AES-256-GCM (an AEAD) with an app-held
// key: a fresh random nonce per Seal, prepended to the ciphertext, and
// authentication that makes a tampered ciphertext or a wrong key fail closed.
//
// The plaintext is never logged and never appears in an error: Open collapses
// every decryption failure into a single ErrDecrypt so nothing about the
// secret leaks through error strings.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// KeySize is the required key length in bytes (AES-256).
const KeySize = 32

// Errors callers compare with errors.Is.
var (
	// ErrKeyLength is a startup/config error: the app-held key is missing or
	// not exactly KeySize bytes.
	ErrKeyLength = fmt.Errorf("secretbox: key must be %d bytes", KeySize)
	// ErrMalformed is returned by Open when the input is too short to contain
	// a nonce -- it is not a valid ciphertext this package produced.
	ErrMalformed = errors.New("secretbox: ciphertext is malformed")
	// ErrDecrypt is returned by Open for any authentication failure: a wrong
	// key or a tampered ciphertext. It carries no detail about the plaintext.
	ErrDecrypt = errors.New("secretbox: decryption failed")
)

// Cipher seals and opens secrets with a fixed app-held key. It is safe for
// concurrent use.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a raw KeySize-byte key. A key of any other length
// is ErrKeyLength -- a startup misconfiguration, not a runtime condition.
func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, ErrKeyLength
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// NewFromHex builds a Cipher from a hex-encoded key (2*KeySize hex digits),
// the form config/env carries it in. A missing (empty) or wrong-length key is
// ErrKeyLength, surfaced as a startup config error by the caller.
func NewFromHex(hexKey string) (*Cipher, error) {
	if hexKey == "" {
		return nil, ErrKeyLength
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("secretbox: decode hex key: %w", err)
	}
	return New(key)
}

// Seal encrypts plaintext, returning nonce||ciphertext||tag. A fresh random
// nonce per call means sealing the same plaintext twice yields different
// outputs.
func (c *Cipher) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secretbox: read nonce: %w", err)
	}
	// Seal appends the ciphertext to nonce, so the nonce prefixes the result
	// and Open can recover it.
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open reverses Seal. It returns ErrMalformed for input too short to hold a
// nonce, and ErrDecrypt for any authentication failure (wrong key or tampered
// ciphertext) -- never a partial or unauthenticated plaintext.
func (c *Cipher) Open(sealed []byte) ([]byte, error) {
	ns := c.aead.NonceSize()
	if len(sealed) < ns {
		return nil, ErrMalformed
	}
	nonce, ciphertext := sealed[:ns], sealed[ns:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
