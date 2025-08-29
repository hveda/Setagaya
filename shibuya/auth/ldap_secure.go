package auth

import (
	"crypto/tls"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rakutentech/shibuya/shibuya/config"
	ldap "gopkg.in/ldap.v2"
)

var (
	CNPatternSecure = regexp.MustCompile(`CN=([^,]+)\,OU=DLM\sDistribution\sGroups`)
)

// SecureLDAPConfig holds secure LDAP configuration
type SecureLDAPConfig struct {
	Server         string
	Port           string
	BaseDN         string
	SystemUser     string
	SystemPassword string
	UseTLS         bool
	SkipTLSVerify  bool
	Timeout        time.Duration
}

// SecureAuthResult contains authentication result with enhanced security
type SecureAuthResult struct {
	ML       []string
	Username string
	Groups   []string
}

// validateInput sanitizes and validates LDAP input to prevent injection attacks
func validateLDAPInput(input string) error {
	// Check for LDAP injection patterns
	dangerousPatterns := []string{
		"*", "(", ")", "\\", "/", "+", "<", ">", "\"", "'", ";", "=", "&", "|", "!",
	}
	
	for _, pattern := range dangerousPatterns {
		if strings.Contains(input, pattern) {
			return fmt.Errorf("invalid characters in input: contains %s", pattern)
		}
	}
	
	// Check length to prevent buffer overflow attempts
	if len(input) > 255 {
		return errors.New("input too long")
	}
	
	// Check for null bytes
	if strings.Contains(input, "\x00") {
		return errors.New("null bytes not allowed")
	}
	
	return nil
}

// AuthSecure performs secure LDAP authentication with TLS and input validation
func AuthSecure(username, password string) (*SecureAuthResult, error) {
	// Input validation
	if err := validateLDAPInput(username); err != nil {
		return nil, fmt.Errorf("invalid username: %w", err)
	}
	
	if len(password) == 0 {
		return nil, errors.New("password cannot be empty")
	}
	
	if len(password) > 512 {
		return nil, errors.New("password too long")
	}

	r := &SecureAuthResult{
		ML:       []string{},
		Username: username,
		Groups:   []string{},
	}
	
	ac := config.SC.AuthConfig
	if ac == nil {
		return nil, errors.New("auth config not initialized")
	}
	
	ldapConfig := &SecureLDAPConfig{
		Server:         ac.LdapServer,
		Port:           ac.LdapPort,
		BaseDN:         ac.BaseDN,
		SystemUser:     ac.SystemUser,
		SystemPassword: ac.SystemPassword,
		UseTLS:         true, // Always use TLS for secure connections
		SkipTLSVerify:  false, // Verify TLS certificates by default
		Timeout:        10 * time.Second,
	}
	
	return authenticateWithSecureLDAP(ldapConfig, username, password, r)
}

// authenticateWithSecureLDAP handles the secure LDAP authentication process
func authenticateWithSecureLDAP(cfg *SecureLDAPConfig, username, password string, result *SecureAuthResult) (*SecureAuthResult, error) {
	// Create secure connection
	l, err := createSecureLDAPConnection(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LDAP server: %w", err)
	}
	defer l.Close()
	
	// Bind with system account
	err = l.Bind(cfg.SystemUser, cfg.SystemPassword)
	if err != nil {
		return nil, fmt.Errorf("system bind failed: %w", err)
	}
	
	// Search for user with sanitized filter
	filter := fmt.Sprintf("(&(objectClass=user)(sAMAccountName=%s))", ldap.EscapeFilter(username))
	searchRequest := ldap.NewSearchRequest(
		cfg.BaseDN,
		ldap.ScopeWholeSubtree, 
		ldap.NeverDerefAliases, 
		1, // Limit results to 1
		int(cfg.Timeout.Seconds()), 
		false,
		filter,
		[]string{"userprincipalname", "memberOf"},
		nil,
	)
	
	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("user search failed: %w", err)
	}
	
	if len(sr.Entries) != 1 {
		return nil, errors.New("user does not exist or multiple users found")
	}
	
	entry := sr.Entries[0]
	
	// Get user principal name
	upnAttr := entry.GetAttributeValue("userprincipalname")
	if upnAttr == "" {
		return nil, errors.New("cannot find user principal name")
	}
	
	// Authenticate user
	err = l.Bind(upnAttr, password)
	if err != nil {
		return nil, errors.New("incorrect password")
	}
	
	// Extract group memberships
	memberOfValues := entry.GetAttributeValues("memberOf")
	for _, membership := range memberOfValues {
		match := CNPatternSecure.FindStringSubmatch(membership)
		if match != nil && len(match) > 1 {
			result.ML = append(result.ML, match[1])
			result.Groups = append(result.Groups, match[1])
		}
	}
	
	return result, nil
}

// createSecureLDAPConnection establishes a secure LDAP connection with TLS
func createSecureLDAPConnection(cfg *SecureLDAPConfig) (*ldap.Conn, error) {
	address := fmt.Sprintf("%s:%s", cfg.Server, cfg.Port)
	
	if cfg.UseTLS {
		// Use LDAPS (LDAP over TLS)
		tlsConfig := &tls.Config{
			ServerName:         cfg.Server,
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}
		
		l, err := ldap.DialTLS("tcp", address, tlsConfig)
		if err != nil {
			return nil, fmt.Errorf("TLS connection failed: %w", err)
		}
		
		// Set connection timeout
		l.SetTimeout(cfg.Timeout)
		return l, nil
	}
	
	// Fallback to regular connection (not recommended for production)
	l, err := ldap.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	
	// Set connection timeout
	l.SetTimeout(cfg.Timeout)
	return l, nil
}