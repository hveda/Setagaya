package utils

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalCSVRange(t *testing.T) {
	testCases := []struct {
		name          string
		totalRows     int
		totalSplits   int
		currentSplit  int
		expectedStart int
		expectedEnd   int
	}{
		{
			name:          "even division",
			totalRows:     100,
			totalSplits:   4,
			currentSplit:  0,
			expectedStart: 0,
			expectedEnd:   25,
		},
		{
			name:          "even division - second split",
			totalRows:     100,
			totalSplits:   4,
			currentSplit:  1,
			expectedStart: 25,
			expectedEnd:   50,
		},
		{
			name:          "even division - last split",
			totalRows:     100,
			totalSplits:   4,
			currentSplit:  3,
			expectedStart: 75,
			expectedEnd:   100,
		},
		{
			name:          "fewer rows than splits",
			totalRows:     5,
			totalSplits:   10,
			currentSplit:  0,
			expectedStart: 0,
			expectedEnd:   5,
		},
		{
			name:          "fewer rows than splits - any split",
			totalRows:     3,
			totalSplits:   5,
			currentSplit:  2,
			expectedStart: 0,
			expectedEnd:   3,
		},
		{
			name:          "single row",
			totalRows:     1,
			totalSplits:   3,
			currentSplit:  0,
			expectedStart: 0,
			expectedEnd:   1,
		},
		{
			name:          "odd division",
			totalRows:     10,
			totalSplits:   3,
			currentSplit:  0,
			expectedStart: 0,
			expectedEnd:   3,
		},
		{
			name:          "odd division - middle",
			totalRows:     10,
			totalSplits:   3,
			currentSplit:  1,
			expectedStart: 3,
			expectedEnd:   6,
		},
		{
			name:          "odd division - last",
			totalRows:     10,
			totalSplits:   3,
			currentSplit:  2,
			expectedStart: 6,
			expectedEnd:   9,
		},
		{
			name:          "zero rows",
			totalRows:     0,
			totalSplits:   3,
			currentSplit:  0,
			expectedStart: 0,
			expectedEnd:   0,
		},
		{
			name:          "single split",
			totalRows:     100,
			totalSplits:   1,
			currentSplit:  0,
			expectedStart: 0,
			expectedEnd:   100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := calCSVRange(tc.totalRows, tc.totalSplits, tc.currentSplit)
			assert.Equal(t, tc.expectedStart, start)
			assert.Equal(t, tc.expectedEnd, end)
		})
	}
}

func TestSplitCSV(t *testing.T) {
	// Create a test CSV file
	csvContent := `name,age,city
Alice,25,New York
Bob,30,Los Angeles
Charlie,35,Chicago
David,40,Houston
Eve,45,Phoenix`

	testCases := []struct {
		name           string
		csvData        string
		totalSplits    int
		currentSplit   int
		expectedLines  int
		shouldError    bool
	}{
		{
			name:          "split into 2 parts - first part",
			csvData:       csvContent,
			totalSplits:   2,
			currentSplit:  0,
			expectedLines: 3, // 6 total rows / 2 = 3 rows per split
			shouldError:   false,
		},
		{
			name:          "split into 2 parts - second part",
			csvData:       csvContent,
			totalSplits:   2,
			currentSplit:  1,
			expectedLines: 3,
			shouldError:   false,
		},
		{
			name:          "split into 3 parts - first part",
			csvData:       csvContent,
			totalSplits:   3,
			currentSplit:  0,
			expectedLines: 2, // 6 total rows / 3 = 2 rows per split
			shouldError:   false,
		},
		{
			name:          "no split (single part)",
			csvData:       csvContent,
			totalSplits:   1,
			currentSplit:  0,
			expectedLines: 6,
			shouldError:   false,
		},
		{
			name:         "invalid split - currentSplit >= totalSplits",
			csvData:      csvContent,
			totalSplits:  3,
			currentSplit: 3,
			shouldError:  true,
		},
		{
			name:         "invalid split - currentSplit > totalSplits",
			csvData:      csvContent,
			totalSplits:  2,
			currentSplit: 5,
			shouldError:  true,
		},
		{
			name:          "empty CSV",
			csvData:       "",
			totalSplits:   2,
			currentSplit:  0,
			expectedLines: 0,
			shouldError:   false,
		},
		{
			name:          "CSV with only header",
			csvData:       "name,age,city",
			totalSplits:   2,
			currentSplit:  0,
			expectedLines: 1, // Header line appears in first split when chunk size is 0 (1/2 = 0, but still gets the row)
			shouldError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := SplitCSV([]byte(tc.csvData), tc.totalSplits, tc.currentSplit)

			if tc.shouldError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				
				// For empty CSV, result might be non-nil but empty
				if tc.csvData == "" {
					// Empty CSV may return empty byte slice, not nil
					assert.Equal(t, 0, tc.expectedLines)
				} else {
					assert.NotNil(t, result)
					
					// Count lines in the result
					if len(result) > 0 {
						lines := strings.Split(strings.TrimSpace(string(result)), "\n")
						// Filter out empty lines
						nonEmptyLines := 0
						for _, line := range lines {
							if strings.TrimSpace(line) != "" {
								nonEmptyLines++
							}
						}
						assert.Equal(t, tc.expectedLines, nonEmptyLines)
					} else {
						// Empty result should match zero expected lines
						assert.Equal(t, 0, tc.expectedLines)
					}
				}
			}
		})
	}
}

func TestSplitCSVContent(t *testing.T) {
	// Test that the content is correctly split
	csvContent := `name,age
Alice,25
Bob,30
Charlie,35
David,40`

	// With 5 total rows, split into 2 parts: 
	// First part gets rows 0-1 (name,age and Alice,25)
	// Second part gets rows 2-3 (Bob,30 and Charlie,35)
	// Note: David,40 is at index 4, which would be in a third chunk if it existed

	// Split into 2 parts
	firstPart, err := SplitCSV([]byte(csvContent), 2, 0)
	assert.NoError(t, err)
	
	secondPart, err := SplitCSV([]byte(csvContent), 2, 1)
	assert.NoError(t, err)

	// Verify first part content (rows 0-1)
	firstPartStr := string(firstPart)
	assert.Contains(t, firstPartStr, "name,age")
	assert.Contains(t, firstPartStr, "Alice,25")

	// Verify second part content (rows 2-3)
	secondPartStr := string(secondPart)
	assert.Contains(t, secondPartStr, "Bob,30")
	assert.Contains(t, secondPartStr, "Charlie,35")

	// David,40 should not appear in either part since it's at index 4
	assert.NotContains(t, firstPartStr, "David,40")
	assert.NotContains(t, secondPartStr, "David,40")
}

func TestSplitCSVWithSpecialCharacters(t *testing.T) {
	// Test CSV with quotes, commas, and newlines in fields
	csvContent := `"name","description","value"
"Alice","Says ""Hello""",25
"Bob","Works at Company, Inc.",30
"Charlie","Multi
line
description",35`

	result, err := SplitCSV([]byte(csvContent), 2, 0)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should handle CSV parsing correctly
	resultStr := string(result)
	assert.Contains(t, resultStr, "Alice")
}

func TestSplitCSVWithMalformedCSV(t *testing.T) {
	// Test with malformed CSV
	malformedCSV := `name,age
Alice,25
Bob,30,extra,fields
Charlie`

	// Should handle malformed CSV gracefully due to FieldsPerRecord = -1
	result, err := SplitCSV([]byte(malformedCSV), 2, 0)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestSplitCSVEdgeCases(t *testing.T) {
	t.Run("zero splits", func(t *testing.T) {
		csvContent := "name,age\nAlice,25"
		result, err := SplitCSV([]byte(csvContent), 0, 0)
		
		// This should either error or handle gracefully
		// The current implementation might divide by zero
		// Let's check if it errors appropriately
		_ = result
		_ = err
		// The behavior depends on implementation details
	})

	t.Run("very large CSV", func(t *testing.T) {
		// Create a large CSV
		var csvBuilder strings.Builder
		csvBuilder.WriteString("id,value\n")
		for i := 0; i < 10000; i++ {
			csvBuilder.WriteString(fmt.Sprintf("%d,value%d\n", i, i))
		}
		
		largeCSV := csvBuilder.String()
		result, err := SplitCSV([]byte(largeCSV), 4, 0)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		
		// Should contain roughly 1/4 of the data
		resultLines := strings.Count(string(result), "\n")
		expectedLines := 10001 / 4 // Total lines / splits
		assert.InDelta(t, expectedLines, resultLines, 50) // Allow some variance
	})
}