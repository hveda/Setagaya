package clusterregistry

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// Ingest tokens authenticate a registered cluster's engine fleet to the
// control plane's ingest endpoint -- per cluster, so one customer's pods hold
// nothing that another customer's pods also hold.
//
// Shape: 32 random bytes (minted by the caller -- randomness is I/O-adjacent
// and lives in the app layer, mirroring phase 10's telemetry seam), carried
// as unpadded base64url (43 characters). At rest, only SHA-256 of the token
// (64 lowercase hex) is kept: a 256-bit random bearer credential needs no
// slow KDF the way a human password does -- lookup is a single indexed
// equality, and there is nothing to grind.

// EncodeToken renders raw mint bytes as the credential a customer holds:
// unpadded base64url, URL-safe and header-safe.
func EncodeToken(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

// HashToken derives the at-rest form of an ingest token: 64 lowercase hex
// characters of SHA-256. Applying it at mint and at ingest makes the stored
// value useless as a credential if the database leaks.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
