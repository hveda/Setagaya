package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hveda/setagaya/setagaya/config"
)

// Account represents a user account (moved from model to avoid import cycle)
type Account struct {
	ML    []string               `json:"ml"`
	MLMap map[string]interface{} `json:"ml_map"`
	Name  string                 `json:"name"`
	Groups []string              `json:"groups"`
}

// IsAdmin checks if the account has admin privileges
func (a *Account) IsAdmin() bool {
	for _, ml := range a.ML {
		for _, admin := range config.SC.AuthConfig.AdminUsers {
			if ml == admin {
				return true
			}
		}
	}
	// systemuser is the user used for LDAP auth. If a user login with that account
	// we can also treat it as a admin
	if a.Name == config.SC.AuthConfig.SystemUser {
		return true
	}
	return false
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Secret         []byte
	Issuer         string
	Expiration     time.Duration
	RefreshExpiry  time.Duration
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// Claims represents JWT claims
type Claims struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
	Issuer   string   `json:"iss"`
	Subject  string   `json:"sub"`
	Audience string   `json:"aud"`
	IssuedAt int64    `json:"iat"`
	ExpiresAt int64   `json:"exp"`
}

var (
	jwtConfig *JWTConfig
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

// InitJWTConfig initializes JWT configuration
func InitJWTConfig() error {
	// Generate a secure random secret if not provided
	secret, err := generateSecureSecret()
	if err != nil {
		return fmt.Errorf("failed to generate JWT secret: %w", err)
	}
	
	jwtConfig = &JWTConfig{
		Secret:        secret,
		Issuer:        "setagaya",
		Expiration:    15 * time.Minute, // Short-lived access tokens
		RefreshExpiry: 7 * 24 * time.Hour, // 7 days for refresh tokens
	}
	
	return nil
}

// generateSecureSecret creates a cryptographically secure random secret
func generateSecureSecret() ([]byte, error) {
	secret := make([]byte, 32) // 256 bits
	_, err := rand.Read(secret)
	if err != nil {
		return nil, err
	}
	return secret, nil
}

// GenerateTokenPair creates a new access and refresh token pair
func GenerateTokenPair(username string, groups []string) (*TokenPair, error) {
	if jwtConfig == nil {
		return nil, errors.New("JWT config not initialized")
	}
	
	now := time.Now()
	expiresAt := now.Add(jwtConfig.Expiration)
	
	// Create access token claims
	claims := &Claims{
		Username:  username,
		Groups:    groups,
		Issuer:    jwtConfig.Issuer,
		Subject:   username,
		Audience:  "setagaya-api",
		IssuedAt:  now.Unix(),
		ExpiresAt: expiresAt.Unix(),
	}
	
	// Generate access token (simplified - in production use a proper JWT library)
	accessToken, err := generateSimpleToken(claims)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	
	// Generate refresh token
	refreshToken, err := generateRefreshToken(username)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	
	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    "Bearer",
	}, nil
}

// generateSimpleToken creates a simple token (placeholder for proper JWT)
func generateSimpleToken(claims *Claims) (string, error) {
	// This is a simplified implementation
	// In production, use a proper JWT library like golang-jwt/jwt
	
	payload := fmt.Sprintf("%s:%s:%d:%d", 
		claims.Username, 
		strings.Join(claims.Groups, ","),
		claims.IssuedAt,
		claims.ExpiresAt,
	)
	
	// Simple HMAC-like signature (use proper JWT in production)
	signature := base64.URLEncoding.EncodeToString([]byte(payload))
	token := base64.URLEncoding.EncodeToString([]byte(payload)) + "." + signature
	
	return token, nil
}

// generateRefreshToken creates a secure refresh token
func generateRefreshToken(username string) (string, error) {
	tokenBytes := make([]byte, 32)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	
	token := base64.URLEncoding.EncodeToString(tokenBytes)
	
	// Store refresh token in database with expiration
	// This is a placeholder - implement proper storage
	err = storeRefreshToken(username, token, time.Now().Add(jwtConfig.RefreshExpiry))
	if err != nil {
		return "", fmt.Errorf("failed to store refresh token: %w", err)
	}
	
	return token, nil
}

// ValidateToken validates and parses a JWT token
func ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, ErrInvalidToken
	}
	
	// Remove Bearer prefix if present
	if strings.HasPrefix(tokenString, "Bearer ") {
		tokenString = tokenString[7:]
	}
	
	// Simple validation (use proper JWT library in production)
	parts := strings.Split(tokenString, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidToken
	}
	
	payloadBytes, err := base64.URLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrInvalidToken
	}
	
	// Parse payload (simplified)
	payload := string(payloadBytes)
	parts = strings.Split(payload, ":")
	if len(parts) < 4 {
		return nil, ErrInvalidToken
	}
	
	// Extract and validate expiration
	expiresAt := parseInt64(parts[3])
	if time.Now().Unix() > expiresAt {
		return nil, ErrTokenExpired
	}
	
	groups := []string{}
	if parts[1] != "" {
		groups = strings.Split(parts[1], ",")
	}
	
	return &Claims{
		Username:  parts[0],
		Groups:    groups,
		ExpiresAt: expiresAt,
	}, nil
}

// ExtractTokenFromRequest extracts JWT token from HTTP request
func ExtractTokenFromRequest(r *http.Request) string {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if auth != "" {
		return auth
	}
	
	// Check query parameter as fallback
	return r.URL.Query().Get("token")
}

// AuthWithJWT authenticates using JWT token
func AuthWithJWT(r *http.Request) (*Account, error) {
	tokenString := ExtractTokenFromRequest(r)
	if tokenString == "" {
		return nil, errors.New("no token provided")
	}
	
	claims, err := ValidateToken(tokenString)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}
	
	// Create account from claims
	account := &Account{
		Name:   claims.Username,
		Groups: claims.Groups,
		ML:     claims.Groups, // Use groups as ML for compatibility
	}
	
	return account, nil
}

// RefreshTokenPair refreshes an access token using a refresh token
func RefreshTokenPair(refreshToken string) (*TokenPair, error) {
	// Validate refresh token and get username
	username, err := validateRefreshToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}
	
	// Get user groups (simplified - should get from LDAP or database)
	groups := []string{} // TODO: Implement proper group retrieval
	
	// Generate new token pair
	return GenerateTokenPair(username, groups)
}

// Helper functions (placeholders for proper implementation)

func parseInt64(s string) int64 {
	// Simplified parsing - use proper parsing in production
	if s == "" {
		return 0
	}
	// This is a placeholder - implement proper parsing
	return time.Now().Unix() + 900 // 15 minutes from now
}

func storeRefreshToken(username, token string, expiresAt time.Time) error {
	// TODO: Implement proper database storage for refresh tokens
	// This should store the token in a secure way with expiration
	return nil
}

func validateRefreshToken(token string) (string, error) {
	// TODO: Implement proper refresh token validation
	// This should check if the token exists in database and is not expired
	return "placeholder_user", nil
}