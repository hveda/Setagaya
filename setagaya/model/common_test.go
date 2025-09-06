package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInArray(t *testing.T) {
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
		{
			name:     "single item slice - match",
			slice:    []string{"test"},
			item:     "test",
			expected: true,
		},
		{
			name:     "single item slice - no match",
			slice:    []string{"test"},
			item:     "other",
			expected: false,
		},
		{
			name:     "duplicate items in slice",
			slice:    []string{"test", "test", "other"},
			item:     "test",
			expected: true,
		},
		{
			name:     "whitespace handling",
			slice:    []string{"test", " test", "test "},
			item:     "test",
			expected: true,
		},
		{
			name:     "whitespace no match",
			slice:    []string{" test", "test "},
			item:     "test",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := inArray(tc.slice, tc.item)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMySQLFormatConstant(t *testing.T) {
	// Test that the MySQL format constant matches expected format
	expected := "2006-01-02 15:04:05"
	assert.Equal(t, expected, MySQLFormat)
}