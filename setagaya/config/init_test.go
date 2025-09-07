package config

import (
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadContext(t *testing.T) {
	testCases := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "local environment",
			envValue: "local",
			expected: "local",
		},
		{
			name:     "production environment",
			envValue: "production",
			expected: "production",
		},
		{
			name:     "test environment",
			envValue: "test",
			expected: "test",
		},
		{
			name:     "empty environment",
			envValue: "",
			expected: "",
		},
		{
			name:     "custom environment",
			envValue: "staging",
			expected: "staging",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set environment variable
			oldEnv := os.Getenv("env")
			defer os.Setenv("env", oldEnv) // Restore after test

			os.Setenv("env", tc.envValue)

			result := loadContext()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMakeHTTPClients(t *testing.T) {
	t.Run("no proxy configuration", func(t *testing.T) {
		sc := &SetagayaConfig{
			HttpConfig: &HttpConfig{
				Proxy: "",
			},
		}

		sc.makeHTTPClients()

		// Both clients should be created
		assert.NotNil(t, sc.HTTPClient)
		assert.NotNil(t, sc.HTTPProxyClient)

		// They should be the same instance when no proxy is configured
		assert.Same(t, sc.HTTPClient, sc.HTTPProxyClient)
	})

	t.Run("with proxy configuration", func(t *testing.T) {
		sc := &SetagayaConfig{
			HttpConfig: &HttpConfig{
				Proxy: "http://proxy.example.com:8080",
			},
		}

		sc.makeHTTPClients()

		// Both clients should be created
		assert.NotNil(t, sc.HTTPClient)
		assert.NotNil(t, sc.HTTPProxyClient)

		// They should be different instances when proxy is configured
		assert.NotSame(t, sc.HTTPClient, sc.HTTPProxyClient)

		// HTTPClient should be a basic client
		assert.Nil(t, sc.HTTPClient.Transport)

		// HTTPProxyClient should have transport configured
		assert.NotNil(t, sc.HTTPProxyClient.Transport)
	})

	t.Run("nil HttpConfig", func(t *testing.T) {
		sc := &SetagayaConfig{
			HttpConfig: nil,
		}

		// This should panic due to nil pointer dereference
		assert.Panics(t, func() {
			sc.makeHTTPClients()
		})
	})

	t.Run("with various proxy URLs", func(t *testing.T) {
		testCases := []struct {
			name       string
			proxyURL   string
			shouldWork bool
		}{
			{
				name:       "http proxy",
				proxyURL:   "http://proxy.example.com:8080",
				shouldWork: true,
			},
			{
				name:       "https proxy",
				proxyURL:   "https://proxy.example.com:8080",
				shouldWork: true,
			},
			{
				name:       "socks5 proxy",
				proxyURL:   "socks5://proxy.example.com:1080",
				shouldWork: true,
			},
			{
				name:       "proxy with auth",
				proxyURL:   "http://user:pass@proxy.example.com:8080",
				shouldWork: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				sc := &SetagayaConfig{
					HttpConfig: &HttpConfig{
						Proxy: tc.proxyURL,
					},
				}

				if tc.shouldWork {
					assert.NotPanics(t, func() {
						sc.makeHTTPClients()
					})
					assert.NotNil(t, sc.HTTPClient)
					assert.NotNil(t, sc.HTTPProxyClient)
					assert.NotSame(t, sc.HTTPClient, sc.HTTPProxyClient)
				}
			})
		}
	})
}

func TestMakeMySQLEndpoint(t *testing.T) {
	testCases := []struct {
		name     string
		config   *MySQLConfig
		expected string
	}{
		{
			name: "basic configuration",
			config: &MySQLConfig{
				Host:     "localhost",
				User:     "root",
				Password: "password",
				Database: "testdb",
			},
			expected: "root:password@tcp(localhost)/testdb?",
		},
		{
			name: "with port",
			config: &MySQLConfig{
				Host:     "localhost:3306",
				User:     "user",
				Password: "pass",
				Database: "mydb",
			},
			expected: "user:pass@tcp(localhost:3306)/mydb?",
		},
		{
			name: "with special characters",
			config: &MySQLConfig{
				Host:     "db.example.com:3306",
				User:     "user@domain",
				Password: "p@ssw0rd!",
				Database: "app_db",
			},
			expected: "user@domain:p@ssw0rd!@tcp(db.example.com:3306)/app_db?",
		},
		{
			name: "empty password",
			config: &MySQLConfig{
				Host:     "localhost",
				User:     "root",
				Password: "",
				Database: "testdb",
			},
			expected: "root:@tcp(localhost)/testdb?",
		},
		{
			name: "minimal config",
			config: &MySQLConfig{
				Host:     "host",
				User:     "u",
				Password: "p",
				Database: "d",
			},
			expected: "u:p@tcp(host)/d?",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeMySQLEndpoint(tc.config)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMySQLConfigStruct(t *testing.T) {
	// Test that MySQLConfig struct can be properly instantiated and used
	config := &MySQLConfig{
		Host:     "localhost",
		User:     "root",
		Password: "password",
		Database: "testdb",
		Keypairs: "keypairs_data",
		Endpoint: "custom_endpoint",
	}

	assert.Equal(t, "localhost", config.Host)
	assert.Equal(t, "root", config.User)
	assert.Equal(t, "password", config.Password)
	assert.Equal(t, "testdb", config.Database)
	assert.Equal(t, "keypairs_data", config.Keypairs)
	assert.Equal(t, "custom_endpoint", config.Endpoint)
}

func TestSetagayaConfigHTTPClients(t *testing.T) {
	// Test that SetagayaConfig properly manages HTTP clients
	config := &SetagayaConfig{}

	// Initially clients should be nil
	assert.Nil(t, config.HTTPClient)
	assert.Nil(t, config.HTTPProxyClient)

	// Test with no proxy config
	config.HttpConfig = &HttpConfig{Proxy: ""}
	config.makeHTTPClients()

	assert.NotNil(t, config.HTTPClient)
	assert.NotNil(t, config.HTTPProxyClient)
	assert.IsType(t, &http.Client{}, config.HTTPClient)
	assert.IsType(t, &http.Client{}, config.HTTPProxyClient)
}

func TestHttpConfigStruct(t *testing.T) {
	// Test HttpConfig struct functionality
	httpConfig := &HttpConfig{
		Proxy: "http://proxy.example.com:8080",
	}

	assert.Equal(t, "http://proxy.example.com:8080", httpConfig.Proxy)

	// Test with empty proxy
	httpConfig.Proxy = ""
	assert.Equal(t, "", httpConfig.Proxy)
}
