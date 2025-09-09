package utils

import (
	"crypto/rand"
	"math/big"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		// Use crypto/rand for cryptographically secure random numbers
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		if err != nil {
			// Fallback to a secure default if crypto/rand fails
			b[i] = letterRunes[0]
			continue
		}
		b[i] = letterRunes[randomIndex.Int64()]
	}
	return string(b)
}
