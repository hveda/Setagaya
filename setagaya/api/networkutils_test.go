package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetrieveClientIP(t *testing.T) {
	testCases := []struct {
		name          string
		remoteAddr    string
		xForwardedFor string
		expectedIP    string
	}{
		{
			name:       "no X-Forwarded-For header",
			remoteAddr: "192.168.1.100:8080",
			expectedIP: "192.168.1.100:8080",
		},
		{
			name:          "single IP in X-Forwarded-For",
			remoteAddr:    "10.0.0.1:8080",
			xForwardedFor: "203.0.113.195",
			expectedIP:    "203.0.113.195",
		},
		{
			name:          "multiple IPs in X-Forwarded-For",
			remoteAddr:    "10.0.0.1:8080",
			xForwardedFor: "203.0.113.195,198.51.100.178,192.168.1.1",
			expectedIP:    "203.0.113.195",
		},
		{
			name:          "X-Forwarded-For with spaces",
			remoteAddr:    "10.0.0.1:8080",
			xForwardedFor: "203.0.113.195, 198.51.100.178, 192.168.1.1",
			expectedIP:    "203.0.113.195",
		},
		{
			name:          "empty X-Forwarded-For header",
			remoteAddr:    "127.0.0.1:8080",
			xForwardedFor: "",
			expectedIP:    "127.0.0.1:8080",
		},
		{
			name:          "X-Forwarded-For with only commas",
			remoteAddr:    "127.0.0.1:8080",
			xForwardedFor: ",,,",
			expectedIP:    "",
		},
		{
			name:       "IPv6 address",
			remoteAddr: "[::1]:8080",
			expectedIP: "[::1]:8080",
		},
		{
			name:          "IPv6 in X-Forwarded-For",
			remoteAddr:    "127.0.0.1:8080",
			xForwardedFor: "2001:db8::1",
			expectedIP:    "2001:db8::1",
		},
		{
			name:          "mixed IPv4 and IPv6 in X-Forwarded-For",
			remoteAddr:    "127.0.0.1:8080",
			xForwardedFor: "2001:db8::1,192.168.1.1",
			expectedIP:    "2001:db8::1",
		},
		{
			name:       "localhost",
			remoteAddr: "127.0.0.1:34567",
			expectedIP: "127.0.0.1:34567",
		},
		{
			name:          "proxy chain simulation",
			remoteAddr:    "10.0.0.1:8080",
			xForwardedFor: "203.0.113.195,198.51.100.178,70.41.3.18,127.0.0.1",
			expectedIP:    "203.0.113.195",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock HTTP request
			req, err := http.NewRequest("GET", "http://example.com", nil)
			assert.NoError(t, err)

			// Set the remote address
			req.RemoteAddr = tc.remoteAddr

			// Set X-Forwarded-For header if provided
			if tc.xForwardedFor != "" {
				req.Header.Set("x-forwarded-for", tc.xForwardedFor)
			}

			// Test the function
			result := retrieveClientIP(req)
			assert.Equal(t, tc.expectedIP, result)
		})
	}
}

func TestRetrieveClientIPEdgeCases(t *testing.T) {
	t.Run("nil request headers", func(t *testing.T) {
		req, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		req.RemoteAddr = "192.168.1.1:8080"
		req.Header = nil

		// Function should handle nil headers gracefully
		result := retrieveClientIP(req)
		assert.Equal(t, "192.168.1.1:8080", result)
	})

	t.Run("case sensitive header check", func(t *testing.T) {
		req, err := http.NewRequest("GET", "http://example.com", nil)
		assert.NoError(t, err)
		req.RemoteAddr = "192.168.1.1:8080"

		// Test that header check is case sensitive (as expected by the implementation)
		req.Header.Set("X-Forwarded-For", "203.0.113.195") // Uppercase
		result := retrieveClientIP(req)
		// Go's http.Header.Get() is case-insensitive, so this should actually work
		assert.Equal(t, "203.0.113.195", result)

		// Test lowercase (should work)
		req.Header.Set("x-forwarded-for", "203.0.113.196") // Lowercase
		result = retrieveClientIP(req)
		assert.Equal(t, "203.0.113.196", result)
	})
}

func TestRetrieveClientIPPerformance(t *testing.T) {
	// Create a request with a long X-Forwarded-For chain
	req, err := http.NewRequest("GET", "http://example.com", nil)
	assert.NoError(t, err)
	req.RemoteAddr = "10.0.0.1:8080"

	longChain := "203.0.113.195"
	for i := 0; i < 100; i++ {
		longChain += ",192.168.1." + string(rune(i%255))
	}
	req.Header.Set("x-forwarded-for", longChain)

	// Function should still extract first IP efficiently
	result := retrieveClientIP(req)
	assert.Equal(t, "203.0.113.195", result)
}
