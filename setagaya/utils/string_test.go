package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandStringRunes(t *testing.T) {
	testCases := []struct {
		name     string
		length   int
		validate func(t *testing.T, result string, length int)
	}{
		{
			name:   "zero length",
			length: 0,
			validate: func(t *testing.T, result string, length int) {
				assert.Equal(t, "", result)
				assert.Equal(t, 0, len(result))
			},
		},
		{
			name:   "single character",
			length: 1,
			validate: func(t *testing.T, result string, length int) {
				assert.Equal(t, 1, len(result))
				assert.True(t, isValidCharacter(result[0]))
			},
		},
		{
			name:   "normal length",
			length: 10,
			validate: func(t *testing.T, result string, length int) {
				assert.Equal(t, 10, len(result))
				for _, char := range result {
					assert.True(t, isValidCharacter(byte(char)))
				}
			},
		},
		{
			name:   "large length",
			length: 1000,
			validate: func(t *testing.T, result string, length int) {
				assert.Equal(t, 1000, len(result))
				for _, char := range result {
					assert.True(t, isValidCharacter(byte(char)))
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := RandStringRunes(tc.length)
			tc.validate(t, result, tc.length)
		})
	}
}

func TestRandStringRunesRandomness(t *testing.T) {
	// Test that multiple calls produce different results (with high probability)
	results := make(map[string]bool)
	length := 20
	attempts := 100

	for i := 0; i < attempts; i++ {
		result := RandStringRunes(length)
		assert.Equal(t, length, len(result))
		results[result] = true
	}

	// With 20 character strings from 52 possible characters, 
	// we should get many unique results
	uniqueResults := len(results)
	assert.Greater(t, uniqueResults, attempts/2, "Should generate mostly unique strings")
}

func TestRandStringRunesCharacterSet(t *testing.T) {
	// Test that only valid characters are used
	length := 1000
	result := RandStringRunes(length)

	for i, char := range result {
		isValid := isValidCharacter(byte(char))
		assert.True(t, isValid, "Character at position %d (%c) should be from valid set", i, char)
	}
}

func TestRandStringRunesDistribution(t *testing.T) {
	// Test that both uppercase and lowercase characters appear
	length := 1000
	result := RandStringRunes(length)

	hasLowercase := false
	hasUppercase := false

	for _, char := range result {
		if char >= 'a' && char <= 'z' {
			hasLowercase = true
		}
		if char >= 'A' && char <= 'Z' {
			hasUppercase = true
		}
	}

	assert.True(t, hasLowercase, "Should contain lowercase characters")
	assert.True(t, hasUppercase, "Should contain uppercase characters")
}

func TestRandStringRunesEdgeCases(t *testing.T) {
	t.Run("negative length", func(t *testing.T) {
		// Negative length should not panic, check how function handles it
		// The function creates make([]rune, n) so negative n will panic
		// This tests the current behavior - if we wanted to handle negatives gracefully,
		// we'd need to modify the function
		assert.Panics(t, func() {
			RandStringRunes(-1)
		})
	})

	t.Run("very large length", func(t *testing.T) {
		// Test with very large length (memory permitting)
		length := 100000
		result := RandStringRunes(length)
		assert.Equal(t, length, len(result))
	})
}

func TestLetterRunesConstant(t *testing.T) {
	// Test that the letterRunes constant contains expected characters
	expectedChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	actualChars := string(letterRunes)
	
	assert.Equal(t, expectedChars, actualChars)
	assert.Equal(t, 52, len(letterRunes))
	
	// Verify no duplicates
	charSet := make(map[rune]bool)
	for _, char := range letterRunes {
		assert.False(t, charSet[char], "Character %c should not be duplicated", char)
		charSet[char] = true
	}
}

// Helper function to check if a character is in the valid set
func isValidCharacter(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

func BenchmarkRandStringRunes(b *testing.B) {
	lengths := []int{1, 10, 100, 1000}
	
	for _, length := range lengths {
		b.Run(fmt.Sprintf("length_%d", length), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				RandStringRunes(length)
			}
		})
	}
}