package auth

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateLDAPInput(t *testing.T) {
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

func TestGenerateSecureSecret(t *testing.T) {
	secret1, err := generateSecureSecret()
	assert.NoError(t, err)
	assert.Len(t, secret1, 32)

	secret2, err := generateSecureSecret()
	assert.NoError(t, err)
	assert.Len(t, secret2, 32)

	// Secrets should be different
	assert.NotEqual(t, secret1, secret2)
}

func TestGenerateSessionID(t *testing.T) {
	id1, err := generateSessionID()
	assert.NoError(t, err)
	assert.NotEmpty(t, id1)

	id2, err := generateSessionID()
	assert.NoError(t, err)
	assert.NotEmpty(t, id2)

	// IDs should be different
	assert.NotEqual(t, id1, id2)
}

func TestGenerateCSRFToken(t *testing.T) {
	token1 := generateCSRFToken()
	assert.NotEmpty(t, token1)

	token2 := generateCSRFToken()
	assert.NotEmpty(t, token2)

	// Tokens should be different
	assert.NotEqual(t, token1, token2)
}

func TestInitJWTConfig(t *testing.T) {
	err := InitJWTConfig()
	assert.NoError(t, err)
	assert.NotNil(t, jwtConfig)
	assert.NotEmpty(t, jwtConfig.Secret)
	assert.Equal(t, "setagaya", jwtConfig.Issuer)
	assert.Equal(t, 15*time.Minute, jwtConfig.Expiration)
}

func TestGenerateTokenPair(t *testing.T) {
	// Initialize JWT config first
	err := InitJWTConfig()
	assert.NoError(t, err)

	username := "testuser"
	groups := []string{"group1", "group2"}

	tokenPair, err := GenerateTokenPair(username, groups)
	assert.NoError(t, err)
	assert.NotNil(t, tokenPair)
	assert.NotEmpty(t, tokenPair.AccessToken)
	assert.NotEmpty(t, tokenPair.RefreshToken)
	assert.Equal(t, "Bearer", tokenPair.TokenType)
	assert.True(t, time.Now().Before(tokenPair.ExpiresAt))
}

func TestValidateToken(t *testing.T) {
	// Initialize JWT config first
	err := InitJWTConfig()
	assert.NoError(t, err)

	username := "testuser"
	groups := []string{"group1", "group2"}

	tokenPair, err := GenerateTokenPair(username, groups)
	assert.NoError(t, err)

	// Test valid token
	claims, err := ValidateToken(tokenPair.AccessToken)
	assert.NoError(t, err)
	assert.Equal(t, username, claims.Username)
	assert.Equal(t, groups, claims.Groups)

	// Test invalid token
	_, err = ValidateToken("invalid.token")
	assert.Error(t, err)

	// Test empty token
	_, err = ValidateToken("")
	assert.Error(t, err)
}

func TestExtractTokenFromRequest(t *testing.T) {
	// Test with Authorization header
	req := &http.Request{
		Header: map[string][]string{
			"Authorization": {"Bearer token123"},
		},
	}
	token := ExtractTokenFromRequest(req)
	assert.Equal(t, "Bearer token123", token)

	// Test with query parameter
	req = &http.Request{
		URL: &url.URL{
			RawQuery: "token=query_token123",
		},
		Header: map[string][]string{},
	}
	token = ExtractTokenFromRequest(req)
	assert.Equal(t, "query_token123", token)

	// Test with no token
	req = &http.Request{
		Header: map[string][]string{},
		URL:    &url.URL{},
	}
	token = ExtractTokenFromRequest(req)
	assert.Empty(t, token)
}

func TestSecureSessionConfig(t *testing.T) {
	err := InitSecureSessionManager()
	assert.NoError(t, err)
	assert.NotNil(t, DefaultSessionManager)
	assert.NotNil(t, DefaultSessionManager.config)
	
	config := DefaultSessionManager.config
	assert.True(t, config.Secure)
	assert.True(t, config.HttpOnly)
	assert.Equal(t, http.SameSiteStrictMode, config.SameSite)
	assert.Equal(t, int(MaxSessionAge.Seconds()), config.MaxAge)
}

// Benchmark tests for performance
func BenchmarkValidateLDAPInput(b *testing.B) {
	input := "validusername123"
	for i := 0; i < b.N; i++ {
		validateLDAPInput(input)
	}
}

func BenchmarkGenerateSecureSecret(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generateSecureSecret()
	}
}

func BenchmarkGenerateSessionID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generateSessionID()
	}
}