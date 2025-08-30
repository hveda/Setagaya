package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateLDAPInputOnly(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{"valid username", "user123", false},
		{"valid username with underscore", "user_name", false},
		{"valid username with dash", "user-name", false},
		{"contains asterisk", "user*", true},
		{"contains parentheses", "user()", true},
		{"contains backslash", "user\\", true},
		{"contains slash", "user/", true},
		{"contains quotes", "user'", true},
		{"contains semicolon", "user;", true},
		{"too long", strings.Repeat("a", 256), true},
		{"contains null byte", "user\x00", true},
		{"empty string", "", false}, // Empty strings are handled separately
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLDAPInput(tt.input)
			if tt.shouldErr {
				assert.Error(t, err, "Expected error for input: %s", tt.input)
			} else {
				assert.NoError(t, err, "Expected no error for input: %s", tt.input)
			}
		})
	}
}

func TestGenerateSecureSecretOnly(t *testing.T) {
	secret1, err := generateSecureSecret()
	assert.NoError(t, err)
	assert.Len(t, secret1, 32)

	secret2, err := generateSecureSecret()
	assert.NoError(t, err)
	assert.Len(t, secret2, 32)

	// Secrets should be different
	assert.NotEqual(t, secret1, secret2)
}