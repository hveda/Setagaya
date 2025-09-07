package object_storage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hveda/Setagaya/setagaya/config"
)

func TestIsProviderGCP(t *testing.T) {
	// Store original config
	originalConfig := config.SC

	testCases := []struct {
		name     string
		provider string
		expected bool
	}{
		{
			name:     "GCP provider",
			provider: "gcp",
			expected: true,
		},
		{
			name:     "Nexus provider",
			provider: "nexus",
			expected: false,
		},
		{
			name:     "Local provider",
			provider: "local",
			expected: false,
		},
		{
			name:     "Empty provider",
			provider: "",
			expected: false,
		},
		{
			name:     "Unknown provider",
			provider: "unknown",
			expected: false,
		},
		{
			name:     "Case sensitive - GCP uppercase",
			provider: "GCP",
			expected: false,
		},
		{
			name:     "Case sensitive - Gcp mixed",
			provider: "Gcp",
			expected: false,
		},
		{
			name:     "Provider with spaces",
			provider: " gcp ",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test config
			config.SC = &config.SetagayaConfig{
				ObjectStorage: &config.ObjectStorage{
					Provider: tc.provider,
				},
			}

			result := IsProviderGCP()
			assert.Equal(t, tc.expected, result)
		})
	}

	// Restore original config
	config.SC = originalConfig
}

func TestIsProviderGCPConstants(t *testing.T) {
	// Test that the constants are correctly defined
	assert.Equal(t, "gcp", gcpStorageProvider)
	assert.Equal(t, "nexus", nexusStorageProvider)
	assert.Equal(t, "local", localStorageProvider)

	// Test that all providers are in the list
	assert.Contains(t, allStorageProvidder, gcpStorageProvider)
	assert.Contains(t, allStorageProvidder, nexusStorageProvider)
	assert.Contains(t, allStorageProvidder, localStorageProvider)
	assert.Equal(t, 3, len(allStorageProvidder))
}

func TestIsProviderGCPWithNilConfig(t *testing.T) {
	// Store original config
	originalConfig := config.SC

	// Test with nil object storage config
	config.SC = &config.SetagayaConfig{
		ObjectStorage: nil,
	}

	// This should panic due to nil pointer dereference
	assert.Panics(t, func() {
		IsProviderGCP()
	})

	// Test with nil setagaya config
	config.SC = nil

	// This should also panic
	assert.Panics(t, func() {
		IsProviderGCP()
	})

	// Restore original config
	config.SC = originalConfig
}
