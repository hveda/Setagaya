package model

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hveda/Setagaya/setagaya/config"
)

func TestGetAccountBySession_NoAuth(t *testing.T) {
	cleanup := SetupTestEnvironment(t)
	defer cleanup()

	// Set no auth mode
	config.SC.AuthConfig.NoAuth = true

	req := httptest.NewRequest("GET", "/test", nil)
	account := GetAccountBySession(req)

	assert.NotNil(t, account)
	assert.Equal(t, "setagaya", account.Name)
	assert.Equal(t, []string{"setagaya"}, account.ML)
	assert.NotNil(t, account.MLMap)
	assert.Contains(t, account.MLMap, "setagaya")
}

func TestGetAccountBySession_WithAuth_NoSession(t *testing.T) {
	// Skip database/session tests in test mode
	if os.Getenv("SETAGAYA_TEST_MODE") == "true" || config.SC.DBC == nil {
		t.Skip("Skipping session test in test mode")
		return
	}

	cleanup := SetupTestEnvironment(t)
	defer cleanup()

	// Set auth mode
	config.SC.AuthConfig.NoAuth = false

	req := httptest.NewRequest("GET", "/test", nil)
	account := GetAccountBySession(req)

	// Should return nil when no session and auth is required
	assert.Nil(t, account)
}

func TestAccount_IsAdmin(t *testing.T) {
	cleanup := SetupTestEnvironment(t)
	defer cleanup()

	testCases := []struct {
		name          string
		account       *Account
		adminUsers    []string
		systemUser    string
		expectedAdmin bool
	}{
		{
			name: "admin user in ML",
			account: &Account{
				Name: "user1",
				ML:   []string{"admin-group", "user-group"},
			},
			adminUsers:    []string{"admin-group"},
			systemUser:    "system",
			expectedAdmin: true,
		},
		{
			name: "non-admin user",
			account: &Account{
				Name: "user1",
				ML:   []string{"user-group", "other-group"},
			},
			adminUsers:    []string{"admin-group"},
			systemUser:    "system",
			expectedAdmin: false,
		},
		{
			name: "system user is admin",
			account: &Account{
				Name: "system",
				ML:   []string{"user-group"},
			},
			adminUsers:    []string{"admin-group"},
			systemUser:    "system",
			expectedAdmin: true,
		},
		{
			name: "empty ML list",
			account: &Account{
				Name: "user1",
				ML:   []string{},
			},
			adminUsers:    []string{"admin-group"},
			systemUser:    "system",
			expectedAdmin: false,
		},
		{
			name: "multiple admin groups",
			account: &Account{
				Name: "user1",
				ML:   []string{"user-group", "admin-group2"},
			},
			adminUsers:    []string{"admin-group1", "admin-group2"},
			systemUser:    "system",
			expectedAdmin: true,
		},
		{
			name: "case sensitive admin check",
			account: &Account{
				Name: "user1",
				ML:   []string{"Admin-Group"},
			},
			adminUsers:    []string{"admin-group"},
			systemUser:    "system",
			expectedAdmin: false,
		},
		{
			name: "empty admin users list",
			account: &Account{
				Name: "user1",
				ML:   []string{"admin-group"},
			},
			adminUsers:    []string{},
			systemUser:    "system",
			expectedAdmin: false,
		},
		{
			name: "system user with admin group",
			account: &Account{
				Name: "system",
				ML:   []string{"admin-group"},
			},
			adminUsers:    []string{"admin-group"},
			systemUser:    "system",
			expectedAdmin: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup config for this test
			config.SC.AuthConfig.AdminUsers = tc.adminUsers
			config.SC.AuthConfig.LdapConfig.SystemUser = tc.systemUser

			result := tc.account.IsAdmin()
			assert.Equal(t, tc.expectedAdmin, result)
		})
	}
}

func TestAccount_MLMap(t *testing.T) {
	cleanup := SetupTestEnvironment(t)
	defer cleanup()

	// Test that MLMap is properly populated in no-auth mode
	config.SC.AuthConfig.NoAuth = true

	req := httptest.NewRequest("GET", "/test", nil)
	account := GetAccountBySession(req)

	assert.NotNil(t, account.MLMap)
	assert.Contains(t, account.MLMap, "setagaya")

	// Test that the map contains the expected value type
	for key, value := range account.MLMap {
		assert.Equal(t, "setagaya", key)
		// The value should be the global 'es' variable (interface{})
		assert.Equal(t, es, value)
	}
}

func TestAccountStruct(t *testing.T) {
	// Test Account struct creation and field access
	account := &Account{
		ML:    []string{"group1", "group2"},
		MLMap: make(map[string]interface{}),
		Name:  "testuser",
	}

	assert.Equal(t, "testuser", account.Name)
	assert.Equal(t, []string{"group1", "group2"}, account.ML)
	assert.NotNil(t, account.MLMap)

	// Test MLMap manipulation
	account.MLMap["test"] = "value"
	assert.Equal(t, "value", account.MLMap["test"])
}

// Test edge cases for session handling
func TestGetAccountBySession_EdgeCases(t *testing.T) {
	// Skip database/session tests in test mode
	if os.Getenv("SETAGAYA_TEST_MODE") == "true" || config.SC.DBC == nil {
		t.Skip("Skipping session test in test mode")
		return
	}

	cleanup := SetupTestEnvironment(t)
	defer cleanup()

	t.Run("nil request", func(t *testing.T) {
		config.SC.AuthConfig.NoAuth = true

		// This should not panic
		account := GetAccountBySession(nil)
		assert.NotNil(t, account)
		assert.Equal(t, "setagaya", account.Name)
	})

	t.Run("empty session key", func(t *testing.T) {
		config.SC.AuthConfig.NoAuth = false
		config.SC.AuthConfig.SessionKey = ""

		req := httptest.NewRequest("GET", "/test", nil)
		account := GetAccountBySession(req)

		// Should handle empty session key gracefully
		assert.Nil(t, account)
	})
}
