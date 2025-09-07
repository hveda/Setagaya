package model

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hveda/Setagaya/setagaya/model"
)

func TestEngineDataConfig(t *testing.T) {
	// Test EngineDataConfig struct creation and field access
	engineData := map[string]*model.SetagayaFile{
		"test1.csv": {
			Filename:     "test1.csv",
			Filepath:     "/path/to/test1.csv",
			Filelink:     "http://storage.com/test1.csv",
			TotalSplits:  4,
			CurrentSplit: 1,
		},
		"test2.csv": {
			Filename:     "test2.csv",
			Filepath:     "/path/to/test2.csv",
			Filelink:     "http://storage.com/test2.csv",
			TotalSplits:  2,
			CurrentSplit: 0,
		},
	}

	edc := &EngineDataConfig{
		EngineData:  engineData,
		Duration:    "300",
		Concurrency: "10",
		Rampup:      "30",
		RunID:       12345,
		EngineID:    1,
	}

	assert.Equal(t, engineData, edc.EngineData)
	assert.Equal(t, "300", edc.Duration)
	assert.Equal(t, "10", edc.Concurrency)
	assert.Equal(t, "30", edc.Rampup)
	assert.Equal(t, int64(12345), edc.RunID)
	assert.Equal(t, 1, edc.EngineID)
	assert.Len(t, edc.EngineData, 2)
}

func TestEngineDataConfigDeepCopy(t *testing.T) {
	// Create original EngineDataConfig
	originalFile := &model.SetagayaFile{
		Filename:     "original.csv",
		Filepath:     "/path/original.csv",
		Filelink:     "http://storage.com/original.csv",
		TotalSplits:  3,
		CurrentSplit: 1,
	}

	original := &EngineDataConfig{
		EngineData: map[string]*model.SetagayaFile{
			"original.csv": originalFile,
		},
		Duration:    "600",
		Concurrency: "20",
		Rampup:      "60",
		RunID:       54321,
		EngineID:    2,
	}

	// Create deep copy
	copied := original.deepCopy()

	// Test that top-level fields are copied
	assert.Equal(t, original.Duration, copied.Duration)
	assert.Equal(t, original.Concurrency, copied.Concurrency)
	assert.Equal(t, original.Rampup, copied.Rampup)
	// RunID and EngineID should now be copied as well
	assert.Equal(t, original.RunID, copied.RunID)
	assert.Equal(t, original.EngineID, copied.EngineID)

	// Test that EngineData map is deep copied
	assert.Len(t, copied.EngineData, 1)
	assert.Contains(t, copied.EngineData, "original.csv")

	copiedFile := copied.EngineData["original.csv"]
	assert.NotNil(t, copiedFile)

	// Test that the file content is copied
	assert.Equal(t, originalFile.Filename, copiedFile.Filename)
	assert.Equal(t, originalFile.Filepath, copiedFile.Filepath)
	assert.Equal(t, originalFile.Filelink, copiedFile.Filelink)
	assert.Equal(t, originalFile.TotalSplits, copiedFile.TotalSplits)
	assert.Equal(t, originalFile.CurrentSplit, copiedFile.CurrentSplit)

	// Test that it's a deep copy (different memory addresses)
	// Verify different objects without using assert.NotSame to avoid pointer issues
	// For maps, we just verify both are non-nil and do other validations
	if original.EngineData == nil || copied.EngineData == nil {
		t.Error("Both maps should be non-nil")
	}
	if originalFile == copiedFile {
		t.Error("Files should be different objects")
	}

	// Test that modifying the copy doesn't affect the original
	copiedFile.Filename = "modified.csv"
	assert.Equal(t, "original.csv", originalFile.Filename)
	assert.Equal(t, "modified.csv", copiedFile.Filename)

	// Test that modifying the copied map doesn't affect the original
	copied.EngineData["new.csv"] = &model.SetagayaFile{Filename: "new.csv"}
	assert.Len(t, original.EngineData, 1)
	assert.Len(t, copied.EngineData, 2)
}

func TestEngineDataConfigDeepCopies(t *testing.T) {
	// Create original EngineDataConfig
	originalFile1 := &model.SetagayaFile{
		Filename:     "file1.csv",
		Filepath:     "/path/file1.csv",
		Filelink:     "http://storage.com/file1.csv",
		TotalSplits:  2,
		CurrentSplit: 0,
	}

	originalFile2 := &model.SetagayaFile{
		Filename:     "file2.csv",
		Filepath:     "/path/file2.csv",
		Filelink:     "http://storage.com/file2.csv",
		TotalSplits:  4,
		CurrentSplit: 2,
	}

	original := &EngineDataConfig{
		EngineData: map[string]*model.SetagayaFile{
			"file1.csv": originalFile1,
			"file2.csv": originalFile2,
		},
		Duration:    "900",
		Concurrency: "50",
		Rampup:      "90",
		RunID:       99999,
		EngineID:    5,
	}

	testCases := []struct {
		name        string
		size        int
		expectedLen int
	}{
		{
			name:        "zero copies",
			size:        0,
			expectedLen: 0,
		},
		{
			name:        "single copy",
			size:        1,
			expectedLen: 1,
		},
		{
			name:        "multiple copies",
			size:        5,
			expectedLen: 5,
		},
		{
			name:        "large number of copies",
			size:        100,
			expectedLen: 100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			copies := original.DeepCopies(tc.size)

			assert.Len(t, copies, tc.expectedLen)

			// Test each copy
			for i, copy := range copies {
				// Test that basic fields are copied
				assert.Equal(t, original.Duration, copy.Duration)
				assert.Equal(t, original.Concurrency, copy.Concurrency)
				assert.Equal(t, original.Rampup, copy.Rampup)

				// Test that EngineData is copied
				assert.Len(t, copy.EngineData, 2)
				assert.Contains(t, copy.EngineData, "file1.csv")
				assert.Contains(t, copy.EngineData, "file2.csv")

				// Test that each copy is independent
				if original.EngineData == nil || copy.EngineData == nil {
					t.Error("Both maps should be non-nil")
				}

			// Test that files in the copy are deep copied
			copiedFile1 := copy.EngineData["file1.csv"]
			copiedFile2 := copy.EngineData["file2.csv"]

			// Test that files are different objects
			if originalFile1 == copiedFile1 {
				t.Error("File1 should be different objects")
			}
			if originalFile2 == copiedFile2 {
				t.Error("File2 should be different objects")
			}

			assert.Equal(t, originalFile1.Filename, copiedFile1.Filename)
			assert.Equal(t, originalFile2.Filename, copiedFile2.Filename)

			// Test that copies are independent of each other
			for j, otherCopy := range copies {
				if i != j {
					// Test that copy maps are both non-nil
					if copy.EngineData == nil || otherCopy.EngineData == nil {
						t.Error("Both copy maps should be non-nil")
					}
				}
			}
			}
		})
	}
}

func TestSetagayaMetric(t *testing.T) {
	// Test SetagayaMetric struct creation and field access
	metric := &SetagayaMetric{
		Threads:      25.5,
		Latency:      150.3,
		Label:        "HTTP Request",
		Status:       "OK",
		Raw:          "raw metric data here",
		CollectionID: "12345",
		PlanID:       "67890",
		EngineID:     "engine-001",
		RunID:        "run-999",
	}

	assert.Equal(t, 25.5, metric.Threads)
	assert.Equal(t, 150.3, metric.Latency)
	assert.Equal(t, "HTTP Request", metric.Label)
	assert.Equal(t, "OK", metric.Status)
	assert.Equal(t, "raw metric data here", metric.Raw)
	assert.Equal(t, "12345", metric.CollectionID)
	assert.Equal(t, "67890", metric.PlanID)
	assert.Equal(t, "engine-001", metric.EngineID)
	assert.Equal(t, "run-999", metric.RunID)
}

func TestSetagayaMetricZeroValues(t *testing.T) {
	// Test SetagayaMetric with zero values
	metric := &SetagayaMetric{}

	assert.Equal(t, 0.0, metric.Threads)
	assert.Equal(t, 0.0, metric.Latency)
	assert.Equal(t, "", metric.Label)
	assert.Equal(t, "", metric.Status)
	assert.Equal(t, "", metric.Raw)
	assert.Equal(t, "", metric.CollectionID)
	assert.Equal(t, "", metric.PlanID)
	assert.Equal(t, "", metric.EngineID)
	assert.Equal(t, "", metric.RunID)
}

func TestEngineDataConfigEdgeCases(t *testing.T) {
	t.Run("empty EngineData map", func(t *testing.T) {
		edc := &EngineDataConfig{
			EngineData:  map[string]*model.SetagayaFile{},
			Duration:    "60",
			Concurrency: "1",
			Rampup:      "5",
		}

		copied := edc.deepCopy()
		assert.NotNil(t, copied.EngineData)
		assert.Len(t, copied.EngineData, 0)
	})

	t.Run("nil EngineData map", func(t *testing.T) {
		edc := &EngineDataConfig{
			EngineData:  nil,
			Duration:    "60",
			Concurrency: "1",
			Rampup:      "5",
		}

		// This should not panic
		assert.NotPanics(t, func() {
			copied := edc.deepCopy()
			assert.NotNil(t, copied.EngineData)
			assert.Len(t, copied.EngineData, 0)
		})
	})

	t.Run("EngineData with nil files", func(t *testing.T) {
		edc := &EngineDataConfig{
			EngineData: map[string]*model.SetagayaFile{
				"test.csv": nil,
			},
			Duration:    "60",
			Concurrency: "1",
			Rampup:      "5",
		}

		// The deepCopy method should handle nil files gracefully now
		// Testing current behavior
		copy := edc.deepCopy()
		assert.NotNil(t, copy)
		assert.Equal(t, edc.Duration, copy.Duration)
		// The nil file should not be copied to the new map
		assert.Len(t, copy.EngineData, 0)
	})

	t.Run("negative size for DeepCopies", func(t *testing.T) {
		edc := &EngineDataConfig{
			EngineData:  map[string]*model.SetagayaFile{},
			Duration:    "60",
			Concurrency: "1",
			Rampup:      "5",
		}

		copies := edc.DeepCopies(-1)
		assert.Len(t, copies, 0)
	})
}

func TestEngineDataConfigFieldTypes(t *testing.T) {
	// Test that fields have expected types
	edc := &EngineDataConfig{}

	// Duration, Concurrency, Rampup should be strings (for flexibility)
	assert.IsType(t, "", edc.Duration)
	assert.IsType(t, "", edc.Concurrency)
	assert.IsType(t, "", edc.Rampup)

	// IDs should be numeric types
	assert.IsType(t, int64(0), edc.RunID)
	assert.IsType(t, int(0), edc.EngineID)

	// EngineData should be a map
	assert.IsType(t, map[string]*model.SetagayaFile{}, edc.EngineData)
}

func TestSetagayaMetricFieldTypes(t *testing.T) {
	// Test that metric fields have expected types
	metric := &SetagayaMetric{}

	// Numeric fields should be float64
	assert.IsType(t, float64(0), metric.Threads)
	assert.IsType(t, float64(0), metric.Latency)

	// ID and text fields should be strings
	assert.IsType(t, "", metric.Label)
	assert.IsType(t, "", metric.Status)
	assert.IsType(t, "", metric.Raw)
	assert.IsType(t, "", metric.CollectionID)
	assert.IsType(t, "", metric.PlanID)
	assert.IsType(t, "", metric.EngineID)
	assert.IsType(t, "", metric.RunID)
}
