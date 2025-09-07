package model

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hveda/Setagaya/setagaya/config"
)

func TestAccountIsAdmin(t *testing.T) {
	// Setup test config
	cleanup := SetupTestEnvironment(t)
	defer cleanup()

	testCases := []struct {
		name        string
		account     *Account
		adminUsers  []string
		systemUser  string
		expectAdmin bool
	}{
		{
			name: "user in admin list",
			account: &Account{
				Name: "admin-user",
				ML:   []string{"admin-group", "user-group"},
			},
			adminUsers:  []string{"admin-group", "super-admin"},
			systemUser:  "system",
			expectAdmin: true,
		},
		{
			name: "user not in admin list",
			account: &Account{
				Name: "regular-user",
				ML:   []string{"user-group", "dev-group"},
			},
			adminUsers:  []string{"admin-group", "super-admin"},
			systemUser:  "system",
			expectAdmin: false,
		},
		{
			name: "system user",
			account: &Account{
				Name: "system",
				ML:   []string{"user-group"},
			},
			adminUsers:  []string{"admin-group"},
			systemUser:  "system",
			expectAdmin: true,
		},
		{
			name: "empty admin list",
			account: &Account{
				Name: "user",
				ML:   []string{"user-group"},
			},
			adminUsers:  []string{},
			systemUser:  "system",
			expectAdmin: false,
		},
		{
			name: "empty ML list",
			account: &Account{
				Name: "user",
				ML:   []string{},
			},
			adminUsers:  []string{"admin-group"},
			systemUser:  "system",
			expectAdmin: false,
		},
		{
			name: "nil ML list",
			account: &Account{
				Name: "user",
				ML:   nil,
			},
			adminUsers:  []string{"admin-group"},
			systemUser:  "system",
			expectAdmin: false,
		},
		{
			name: "multiple admin groups",
			account: &Account{
				Name: "user",
				ML:   []string{"group1", "admin2", "group3"},
			},
			adminUsers:  []string{"admin1", "admin2", "admin3"},
			systemUser:  "system",
			expectAdmin: true,
		},
		{
			name: "case sensitive admin check",
			account: &Account{
				Name: "user",
				ML:   []string{"Admin-Group"},
			},
			adminUsers:  []string{"admin-group"}, // Different case
			systemUser:  "system",
			expectAdmin: false,
		},
		{
			name: "case sensitive system user",
			account: &Account{
				Name: "System", // Different case
				ML:   []string{"user-group"},
			},
			adminUsers:  []string{"admin-group"},
			systemUser:  "system",
			expectAdmin: false,
		},
		{
			name: "empty system user",
			account: &Account{
				Name: "",
				ML:   []string{"user-group"},
			},
			adminUsers:  []string{"admin-group"},
			systemUser:  "",
			expectAdmin: true, // Empty name matches empty system user
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test config with specific admin users and system user
			config.SC.AuthConfig.AdminUsers = tc.adminUsers
			config.SC.AuthConfig.SystemUser = tc.systemUser

			result := tc.account.IsAdmin()
			assert.Equal(t, tc.expectAdmin, result)
		})
	}
}

func TestAccountIsAdminEdgeCases(t *testing.T) {
	// Setup test config
	cleanup := SetupTestEnvironment(t)
	defer cleanup()

	t.Run("nil account", func(t *testing.T) {
		var account *Account = nil

		// This should panic
		assert.Panics(t, func() {
			account.IsAdmin()
		})
	})

	t.Run("whitespace in names", func(t *testing.T) {
		config.SC.AuthConfig.AdminUsers = []string{" admin-group "}
		config.SC.AuthConfig.SystemUser = " system "

		account := &Account{
			Name: " system ",
			ML:   []string{" admin-group "},
		}

		// Should be true for system user (exact match)
		result := account.IsAdmin()
		assert.True(t, result)
	})

	t.Run("special characters", func(t *testing.T) {
		config.SC.AuthConfig.AdminUsers = []string{"admin@group.com", "super-admin!"}
		config.SC.AuthConfig.SystemUser = "system$user"

		account := &Account{
			Name: "regular-user",
			ML:   []string{"admin@group.com"},
		}

		result := account.IsAdmin()
		assert.True(t, result)
	})

	t.Run("very long names", func(t *testing.T) {
		longGroupName := make([]byte, 1000)
		for i := range longGroupName {
			longGroupName[i] = 'a'
		}
		longName := string(longGroupName)

		config.SC.AuthConfig.AdminUsers = []string{longName}
		config.SC.AuthConfig.SystemUser = "system"

		account := &Account{
			Name: "user",
			ML:   []string{longName},
		}

		result := account.IsAdmin()
		assert.True(t, result)
	})
}

func TestAccountStructEnhanced(t *testing.T) {
	// Test Account struct initialization and field access
	account := &Account{
		Name: "test-user",
		ML:   []string{"group1", "group2"},
		MLMap: map[string]interface{}{
			"group1": nil,
			"group2": "some-value",
		},
	}

	assert.Equal(t, "test-user", account.Name)
	assert.Equal(t, 2, len(account.ML))
	assert.Contains(t, account.ML, "group1")
	assert.Contains(t, account.ML, "group2")
	assert.Equal(t, 2, len(account.MLMap))
	assert.Contains(t, account.MLMap, "group1")
	assert.Contains(t, account.MLMap, "group2")
}

func TestAccountWithEmptyValues(t *testing.T) {
	// Test Account with empty/nil values
	account := &Account{}

	assert.Equal(t, "", account.Name)
	assert.Nil(t, account.ML)
	assert.Nil(t, account.MLMap)

	// Should not panic when calling IsAdmin on empty account
	cleanup := SetupTestEnvironment(t)
	defer cleanup()

	config.SC.AuthConfig.AdminUsers = []string{"admin"}
	config.SC.AuthConfig.SystemUser = "system"

	assert.NotPanics(t, func() {
		result := account.IsAdmin()
		assert.False(t, result)
	})
}
