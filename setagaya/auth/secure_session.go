package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/sessions"
	log "github.com/sirupsen/logrus"
)

const (
	DefaultSessionName = "setagaya-session"
	MaxSessionAge      = 4 * time.Hour // Reduced from 1 year to 4 hours
	SessionIDLength    = 32
)

// SecureSessionConfig holds secure session configuration
type SecureSessionConfig struct {
	Name         string
	MaxAge       int
	Secure       bool
	HttpOnly     bool
	SameSite     http.SameSite
	Domain       string
	Path         string
	EncryptionKey []byte
	SigningKey    []byte
}

// SecureSessionManager manages secure sessions
type SecureSessionManager struct {
	store  sessions.Store
	config *SecureSessionConfig
}

var (
	DefaultSessionManager *SecureSessionManager
	ErrSessionNotFound    = errors.New("session not found")
	ErrSessionExpired     = errors.New("session expired")
	ErrInvalidSession     = errors.New("invalid session")
)

// InitSecureSessionManager initializes the secure session manager
func InitSecureSessionManager() error {
	encryptionKey, err := generateSecureKey(32) // 256-bit encryption key
	if err != nil {
		return fmt.Errorf("failed to generate encryption key: %w", err)
	}
	
	signingKey, err := generateSecureKey(64) // 512-bit signing key
	if err != nil {
		return fmt.Errorf("failed to generate signing key: %w", err)
	}
	
	sessionConfig := &SecureSessionConfig{
		Name:          DefaultSessionName,
		MaxAge:        int(MaxSessionAge.Seconds()),
		Secure:        true,  // Only send over HTTPS
		HttpOnly:      true,  // Prevent XSS attacks
		SameSite:      http.SameSiteStrictMode, // CSRF protection
		Domain:        "",    // Same domain only
		Path:          "/",
		EncryptionKey: encryptionKey,
		SigningKey:    signingKey,
	}
	
	// Create secure cookie store
	store := sessions.NewCookieStore(signingKey, encryptionKey)
	store.Options = &sessions.Options{
		Path:     sessionConfig.Path,
		Domain:   sessionConfig.Domain,
		MaxAge:   sessionConfig.MaxAge,
		Secure:   sessionConfig.Secure,
		HttpOnly: sessionConfig.HttpOnly,
		SameSite: sessionConfig.SameSite,
	}
	
	DefaultSessionManager = &SecureSessionManager{
		store:  store,
		config: sessionConfig,
	}
	
	log.Info("Secure session manager initialized")
	return nil
}

// generateSecureKey generates a cryptographically secure random key
func generateSecureKey(length int) ([]byte, error) {
	key := make([]byte, length)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// CreateSession creates a new secure session
func (sm *SecureSessionManager) CreateSession(w http.ResponseWriter, r *http.Request, username string, groups []string) error {
	session, err := sm.store.Get(r, sm.config.Name)
	if err != nil {
		log.WithError(err).Warn("Failed to get session, creating new one")
	}
	
	// Generate secure session ID
	sessionID, err := generateSessionID()
	if err != nil {
		return fmt.Errorf("failed to generate session ID: %w", err)
	}
	
	// Set session values with security measures
	session.Values["id"] = sessionID
	session.Values["username"] = username
	session.Values["groups"] = groups
	session.Values["created_at"] = time.Now()
	session.Values["last_activity"] = time.Now()
	session.Values["csrf_token"] = generateCSRFToken()
	
	// Force session regeneration for security
	session.Options = &sessions.Options{
		Path:     sm.config.Path,
		Domain:   sm.config.Domain,
		MaxAge:   sm.config.MaxAge,
		Secure:   sm.config.Secure,
		HttpOnly: sm.config.HttpOnly,
		SameSite: sm.config.SameSite,
	}
	
	err = session.Save(r, w)
	if err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}
	
	log.WithFields(log.Fields{
		"username":   username,
		"session_id": sessionID,
	}).Info("Secure session created")
	
	return nil
}

// GetSession retrieves and validates a session
func (sm *SecureSessionManager) GetSession(r *http.Request) (*SessionData, error) {
	session, err := sm.store.Get(r, sm.config.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	
	if session.IsNew {
		return nil, ErrSessionNotFound
	}
	
	// Validate session data
	sessionData, err := sm.validateSessionData(session)
	if err != nil {
		return nil, err
	}
	
	// Check session expiry
	if sm.isSessionExpired(sessionData) {
		sm.DestroySession(nil, r, session) // Best effort cleanup
		return nil, ErrSessionExpired
	}
	
	// Update last activity
	session.Values["last_activity"] = time.Now()
	
	return sessionData, nil
}

// SessionData represents validated session information
type SessionData struct {
	ID           string
	Username     string
	Groups       []string
	CreatedAt    time.Time
	LastActivity time.Time
	CSRFToken    string
}

// validateSessionData validates and extracts session data
func (sm *SecureSessionManager) validateSessionData(session *sessions.Session) (*SessionData, error) {
	// Extract and validate session ID
	sessionID, ok := session.Values["id"].(string)
	if !ok || sessionID == "" {
		return nil, ErrInvalidSession
	}
	
	// Extract username
	username, ok := session.Values["username"].(string)
	if !ok || username == "" {
		return nil, ErrInvalidSession
	}
	
	// Extract groups
	groups, ok := session.Values["groups"].([]string)
	if !ok {
		groups = []string{}
	}
	
	// Extract timestamps
	createdAt, ok := session.Values["created_at"].(time.Time)
	if !ok {
		return nil, ErrInvalidSession
	}
	
	lastActivity, ok := session.Values["last_activity"].(time.Time)
	if !ok {
		lastActivity = createdAt
	}
	
	// Extract CSRF token
	csrfToken, ok := session.Values["csrf_token"].(string)
	if !ok {
		csrfToken = ""
	}
	
	return &SessionData{
		ID:           sessionID,
		Username:     username,
		Groups:       groups,
		CreatedAt:    createdAt,
		LastActivity: lastActivity,
		CSRFToken:    csrfToken,
	}, nil
}

// isSessionExpired checks if a session has expired
func (sm *SecureSessionManager) isSessionExpired(data *SessionData) bool {
	maxAge := time.Duration(sm.config.MaxAge) * time.Second
	return time.Since(data.LastActivity) > maxAge
}

// DestroySession securely destroys a session
func (sm *SecureSessionManager) DestroySession(w http.ResponseWriter, r *http.Request, session *sessions.Session) error {
	if session == nil {
		var err error
		session, err = sm.store.Get(r, sm.config.Name)
		if err != nil {
			return err
		}
	}
	
	// Clear session values
	for key := range session.Values {
		delete(session.Values, key)
	}
	
	// Set MaxAge to -1 to delete the cookie
	session.Options.MaxAge = -1
	
	if w != nil {
		err := session.Save(r, w)
		if err != nil {
			return fmt.Errorf("failed to destroy session: %w", err)
		}
	}
	
	log.Info("Session destroyed")
	return nil
}

// ValidateCSRFToken validates CSRF token
func (sm *SecureSessionManager) ValidateCSRFToken(r *http.Request, token string) bool {
	sessionData, err := sm.GetSession(r)
	if err != nil {
		return false
	}
	
	// Use constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(sessionData.CSRFToken), []byte(token)) == 1
}

// RegenerateSession regenerates session ID for security
func (sm *SecureSessionManager) RegenerateSession(w http.ResponseWriter, r *http.Request) error {
	// Get current session data
	sessionData, err := sm.GetSession(r)
	if err != nil {
		return err
	}
	
	// Get current session
	session, err := sm.store.Get(r, sm.config.Name)
	if err != nil {
		return err
	}
	
	// Destroy old session
	err = sm.DestroySession(w, r, session)
	if err != nil {
		log.WithError(err).Warn("Failed to destroy old session during regeneration")
	}
	
	// Create new session with same data
	return sm.CreateSession(w, r, sessionData.Username, sessionData.Groups)
}

// Helper functions

func generateSessionID() (string, error) {
	bytes := make([]byte, SessionIDLength)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func generateCSRFToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)
}