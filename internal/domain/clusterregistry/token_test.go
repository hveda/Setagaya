package clusterregistry

import (
	"encoding/base64"
	"encoding/hex"
	"testing"
)

// The token pair is a contract between three callers -- the minting
// registration, the at-rest column, and the ingest lookup -- so its shape is
// pinned here rather than in any one of them.
func TestToken_EncodeAndHash(t *testing.T) {
	t.Parallel()

	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}

	tok := EncodeToken(raw)
	if len(tok) != 43 { // 32 bytes -> 43 unpadded base64url chars
		t.Fatalf("token len = %d (%q), want 43", len(tok), tok)
	}
	if _, err := base64.RawURLEncoding.DecodeString(tok); err != nil {
		t.Fatalf("token is not unpadded base64url: %v", err)
	}

	h := HashToken(tok)
	if len(h) != 64 {
		t.Fatalf("hash len = %d, want 64 hex chars", len(h))
	}
	if _, err := hex.DecodeString(h); err != nil {
		t.Fatalf("hash is not hex: %v", err)
	}

	// Deterministic, and distinct tokens hash distinctly.
	if HashToken(tok) != h {
		t.Fatal("HashToken is not deterministic")
	}
	if HashToken(tok+"x") == h {
		t.Fatal("a different token hashed to the same value")
	}
}
