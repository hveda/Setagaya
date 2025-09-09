package model

import (
	"fmt"
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
		{
			name:     "nil slice",
			slice:    nil,
			item:     "apple",
			expected: false,
		},
		{
			name:     "special characters",
			slice:    []string{"test@example.com", "user$name", "file.txt"},
			item:     "test@example.com",
			expected: true,
		},
		{
			name:     "unicode characters",
			slice:    []string{"测试", "тест", "テスト"},
			item:     "测试",
			expected: true,
		},
		{
			name:     "numbers as strings",
			slice:    []string{"123", "456", "789"},
			item:     "456",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := inArray(tc.slice, tc.item)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestInArray_Performance(t *testing.T) {
	// Test performance with large slice
	largeSlice := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		largeSlice[i] = fmt.Sprintf("item-%d", i)
	}

	// Test finding item at beginning
	result := inArray(largeSlice, "item-0")
	assert.True(t, result)

	// Test finding item at end
	result = inArray(largeSlice, "item-999")
	assert.True(t, result)

	// Test finding item in middle
	result = inArray(largeSlice, "item-500")
	assert.True(t, result)

	// Test not finding item
	result = inArray(largeSlice, "item-1000")
	assert.False(t, result)
}

func TestInArray_EdgeCases(t *testing.T) {
	// Test with very long strings
	longString := make([]byte, 1000)
	for i := range longString {
		longString[i] = 'a'
	}
	longStr := string(longString)
	
	slice := []string{longStr, "short"}
	result := inArray(slice, longStr)
	assert.True(t, result)

	// Test with empty strings in slice
	emptySlice := []string{"", "", "test", ""}
	result = inArray(emptySlice, "")
	assert.True(t, result)

	result = inArray(emptySlice, "test")
	assert.True(t, result)

	result = inArray(emptySlice, "notfound")
	assert.False(t, result)
}

func TestMySQLFormatConstant(t *testing.T) {
	// Test that the MySQL format constant matches expected format
	expected := "2006-01-02 15:04:05"
	assert.Equal(t, expected, MySQLFormat)
}
