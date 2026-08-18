package secretbox_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/adapters/secretbox"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, secretbox.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand key: %v", err)
	}
	return key
}

func TestSealOpen_RoundTrips(t *testing.T) {
	t.Parallel()
	c, err := secretbox.New(newKey(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	plaintext := []byte("apiVersion: v1\nkind: Config\n... a kubeconfig ...")

	sealed, err := c.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatalf("Seal output contains the plaintext verbatim")
	}
	got, err := c.Open(sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("Open = %q, want %q", got, plaintext)
	}
}

// A fresh nonce per Seal means the same plaintext seals to distinct outputs.
func TestSeal_NonceIsRandomPerCall(t *testing.T) {
	t.Parallel()
	c, _ := secretbox.New(newKey(t))
	plaintext := []byte("same secret")
	a, _ := c.Seal(plaintext)
	b, _ := c.Seal(plaintext)
	if bytes.Equal(a, b) {
		t.Fatalf("two seals of identical plaintext are equal; nonce not random")
	}
}

func TestOpen_TamperedFailsClosed(t *testing.T) {
	t.Parallel()
	c, _ := secretbox.New(newKey(t))
	sealed, _ := c.Seal([]byte("secret"))

	// Flip the last byte (inside the GCM tag / ciphertext).
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0xff

	got, err := c.Open(tampered)
	if !errors.Is(err, secretbox.ErrDecrypt) {
		t.Fatalf("Open(tampered) err = %v, want ErrDecrypt", err)
	}
	if got != nil {
		t.Fatalf("Open(tampered) returned %q, want nil (no unauthenticated plaintext)", got)
	}
}

func TestOpen_WrongKeyFailsClosed(t *testing.T) {
	t.Parallel()
	enc, _ := secretbox.New(newKey(t))
	sealed, _ := enc.Seal([]byte("secret"))

	dec, _ := secretbox.New(newKey(t)) // different key
	if _, err := dec.Open(sealed); !errors.Is(err, secretbox.ErrDecrypt) {
		t.Fatalf("Open(wrong key) err = %v, want ErrDecrypt", err)
	}
}

func TestOpen_MalformedTooShort(t *testing.T) {
	t.Parallel()
	c, _ := secretbox.New(newKey(t))
	if _, err := c.Open([]byte{0x00, 0x01}); !errors.Is(err, secretbox.ErrMalformed) {
		t.Fatalf("Open(short) err = %v, want ErrMalformed", err)
	}
}

// The plaintext must never leak through an error string.
func TestOpen_ErrorHasNoPlaintext(t *testing.T) {
	t.Parallel()
	enc, _ := secretbox.New(newKey(t))
	secret := "super-secret-token-abc123"
	sealed, _ := enc.Seal([]byte(secret))
	dec, _ := secretbox.New(newKey(t))
	_, err := dec.Open(sealed)
	if err == nil {
		t.Fatal("Open(wrong key) unexpectedly succeeded")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error string leaks plaintext: %q", err.Error())
	}
}

func TestNew_KeyLength(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  []byte
	}{
		{"empty", nil},
		{"short", make([]byte, 16)},
		{"long", make([]byte, 33)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := secretbox.New(tc.key); !errors.Is(err, secretbox.ErrKeyLength) {
				t.Fatalf("New(%s) err = %v, want ErrKeyLength", tc.name, err)
			}
		})
	}
}

func TestNewFromHex(t *testing.T) {
	t.Parallel()

	// A valid 64-hex-digit key round-trips through Seal/Open.
	raw := newKey(t)
	hexKey := ""
	for _, b := range raw {
		const digits = "0123456789abcdef"
		hexKey += string(digits[b>>4]) + string(digits[b&0x0f])
	}
	c, err := secretbox.NewFromHex(hexKey)
	if err != nil {
		t.Fatalf("NewFromHex(valid): %v", err)
	}
	sealed, _ := c.Seal([]byte("x"))
	if _, err := c.Open(sealed); err != nil {
		t.Fatalf("round-trip via NewFromHex: %v", err)
	}

	if _, err := secretbox.NewFromHex(""); !errors.Is(err, secretbox.ErrKeyLength) {
		t.Fatalf("NewFromHex(empty) err = %v, want ErrKeyLength", err)
	}
	if _, err := secretbox.NewFromHex("abcd"); !errors.Is(err, secretbox.ErrKeyLength) {
		t.Fatalf("NewFromHex(short) err = %v, want ErrKeyLength", err)
	}
	if _, err := secretbox.NewFromHex("nothex!!"); err == nil {
		t.Fatalf("NewFromHex(non-hex) err = nil, want decode error")
	}
}
