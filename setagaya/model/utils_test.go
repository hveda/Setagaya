package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test the pure utility functions that don't depend on config
func TestUtilityFunctions(t *testing.T) {
	t.Run("inArray function", func(t *testing.T) {
		testCases := []struct {
			name     string
			slice    []string
			item     string
			expected bool
		}{
			{
				name:     "item exists in slice",
				slice:    []string{"apple", "banana", "cherry"},
				item:     "banana",
				expected: true,
			},
			{
				name:     "item does not exist in slice",
				slice:    []string{"apple", "banana", "cherry"},
				item:     "grape",
				expected: false,
			},
			{
				name:     "empty slice",
				slice:    []string{},
				item:     "apple",
				expected: false,
			},
			{
				name:     "empty string in slice",
				slice:    []string{"", "apple", "banana"},
				item:     "",
				expected: true,
			},
			{
				name:     "case sensitive match",
				slice:    []string{"Apple", "Banana", "Cherry"},
				item:     "apple",
				expected: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := inArray(tc.slice, tc.item)
				assert.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("MySQL format constant", func(t *testing.T) {
		expected := "2006-01-02 15:04:05"
		assert.Equal(t, expected, MySQLFormat)
	})
}
