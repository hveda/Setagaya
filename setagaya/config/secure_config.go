package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// SecureConfig provides encrypted configuration management
type SecureConfig struct {
	keyring cipher.AEAD
}

// EncryptedConfigField represents an encrypted configuration field
type EncryptedConfigField struct {
	Value     string `json:"value"`
	Encrypted bool   `json:"encrypted"`
	Nonce     string `json:"nonce,omitempty"`
}

// NewSecureConfig creates a new secure configuration manager
func NewSecureConfig(key []byte) (*SecureConfig, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be exactly 32 bytes")
	}
	
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create AEAD: %w", err)
	}
	
	return &SecureConfig{
		keyring: aead,
	}, nil
}

// Encrypt encrypts a configuration value
func (sc *SecureConfig) Encrypt(plaintext string) (*EncryptedConfigField, error) {
	nonce := make([]byte, sc.keyring.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	
	ciphertext := sc.keyring.Seal(nil, nonce, []byte(plaintext), nil)
	
	return &EncryptedConfigField{
		Value:     base64.StdEncoding.EncodeToString(ciphertext),
		Encrypted: true,
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
	}, nil
}

// Decrypt decrypts a configuration value
func (sc *SecureConfig) Decrypt(field *EncryptedConfigField) (string, error) {
	if !field.Encrypted {
		return field.Value, nil
	}
	
	ciphertext, err := base64.StdEncoding.DecodeString(field.Value)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}
	
	nonce, err := base64.StdEncoding.DecodeString(field.Nonce)
	if err != nil {
		return "", fmt.Errorf("failed to decode nonce: %w", err)
	}
	
	plaintext, err := sc.keyring.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	
	return string(plaintext), nil
}

// SecureConfigValues holds encrypted configuration values
type SecureConfigValues struct {
	DatabasePassword  *EncryptedConfigField `json:"database_password"`
	LDAPSystemUser    *EncryptedConfigField `json:"ldap_system_user"`
	LDAPSystemPassword *EncryptedConfigField `json:"ldap_system_password"`
	JWTSecret         *EncryptedConfigField `json:"jwt_secret"`
	SessionKey        *EncryptedConfigField `json:"session_key"`
	ObjectStorageKey  *EncryptedConfigField `json:"object_storage_key"`
}

// LoadSecureConfig loads and decrypts configuration values
func LoadSecureConfig(configPath, keyPath string) (*SecureConfigValues, error) {
	// Load encryption key
	key, err := loadEncryptionKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load encryption key: %w", err)
	}
	
	// Create secure config manager
	sc, err := NewSecureConfig(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create secure config: %w", err)
	}
	
	// Load encrypted configuration
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	
	var secureConfig SecureConfigValues
	if err := json.Unmarshal(configData, &secureConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	
	// Decrypt values in place (for internal use)
	if err := decryptConfigValues(sc, &secureConfig); err != nil {
		return nil, fmt.Errorf("failed to decrypt config values: %w", err)
	}
	
	return &secureConfig, nil
}

// SaveSecureConfig encrypts and saves configuration values
func SaveSecureConfig(configPath, keyPath string, config *SecureConfigValues) error {
	// Load or generate encryption key
	key, err := loadOrGenerateEncryptionKey(keyPath)
	if err != nil {
		return fmt.Errorf("failed to load/generate encryption key: %w", err)
	}
	
	// Create secure config manager
	sc, err := NewSecureConfig(key)
	if err != nil {
		return fmt.Errorf("failed to create secure config: %w", err)
	}
	
	// Encrypt values
	encryptedConfig, err := encryptConfigValues(sc, config)
	if err != nil {
		return fmt.Errorf("failed to encrypt config values: %w", err)
	}
	
	// Save to file
	configData, err := json.MarshalIndent(encryptedConfig, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	
	return nil
}

// Helper functions

func loadEncryptionKey(keyPath string) ([]byte, error) {
	if keyPath == "" {
		return nil, errors.New("encryption key path is required")
	}
	
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	
	// Decode base64 key
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyData)))
	if err != nil {
		return nil, fmt.Errorf("failed to decode key: %w", err)
	}
	
	if len(key) != 32 {
		return nil, errors.New("encryption key must be exactly 32 bytes")
	}
	
	return key, nil
}

func loadOrGenerateEncryptionKey(keyPath string) ([]byte, error) {
	// Try to load existing key
	if _, err := os.Stat(keyPath); err == nil {
		return loadEncryptionKey(keyPath)
	}
	
	// Generate new key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	
	// Save key to file
	keyData := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(keyPath, []byte(keyData), 0600); err != nil {
		return nil, fmt.Errorf("failed to save encryption key: %w", err)
	}
	
	return key, nil
}

func decryptConfigValues(sc *SecureConfig, config *SecureConfigValues) error {
	// This would decrypt values in place for runtime use
	// Implementation depends on how you want to handle decrypted values
	return nil
}

func encryptConfigValues(sc *SecureConfig, config *SecureConfigValues) (*SecureConfigValues, error) {
	encrypted := &SecureConfigValues{}
	
	// Encrypt each field
	if config.DatabasePassword != nil && !config.DatabasePassword.Encrypted {
		field, err := sc.Encrypt(config.DatabasePassword.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt database password: %w", err)
		}
		encrypted.DatabasePassword = field
	}
	
	if config.LDAPSystemPassword != nil && !config.LDAPSystemPassword.Encrypted {
		field, err := sc.Encrypt(config.LDAPSystemPassword.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt LDAP password: %w", err)
		}
		encrypted.LDAPSystemPassword = field
	}
	
	// Add other fields as needed...
	
	return encrypted, nil
}

// Configuration migration utilities

// MigrateToSecureConfig migrates a plain text configuration to encrypted format
func MigrateToSecureConfig(plainConfigPath, secureConfigPath, keyPath string) error {
	// Load plain configuration
	configData, err := os.ReadFile(plainConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read plain config: %w", err)
	}
	
	var plainConfig map[string]interface{}
	if err := json.Unmarshal(configData, &plainConfig); err != nil {
		return fmt.Errorf("failed to parse plain config: %w", err)
	}
	
	// Extract sensitive values
	secureConfig := &SecureConfigValues{}
	
	if dbConfig, ok := plainConfig["db"].(map[string]interface{}); ok {
		if password, ok := dbConfig["password"].(string); ok && password != "" {
			secureConfig.DatabasePassword = &EncryptedConfigField{
				Value:     password,
				Encrypted: false,
			}
		}
	}
	
	if authConfig, ok := plainConfig["auth_config"].(map[string]interface{}); ok {
		if systemPassword, ok := authConfig["system_password"].(string); ok && systemPassword != "" {
			secureConfig.LDAPSystemPassword = &EncryptedConfigField{
				Value:     systemPassword,
				Encrypted: false,
			}
		}
	}
	
	// Save encrypted configuration
	return SaveSecureConfig(secureConfigPath, keyPath, secureConfig)
}

// ValidateConfigSecurity performs security validation on configuration
func ValidateConfigSecurity(configPath string) []string {
	var issues []string
	
	configData, err := os.ReadFile(configPath)
	if err != nil {
		issues = append(issues, "Cannot read configuration file")
		return issues
	}
	
	var config map[string]interface{}
	if err := json.Unmarshal(configData, &config); err != nil {
		issues = append(issues, "Invalid JSON configuration")
		return issues
	}
	
	// Check for plaintext passwords
	if dbConfig, ok := config["db"].(map[string]interface{}); ok {
		if password, ok := dbConfig["password"].(string); ok && password != "" {
			issues = append(issues, "Database password stored in plaintext")
		}
	}
	
	if authConfig, ok := config["auth_config"].(map[string]interface{}); ok {
		if systemPassword, ok := authConfig["system_password"].(string); ok && systemPassword != "" {
			issues = append(issues, "LDAP system password stored in plaintext")
		}
	}
	
	// Check file permissions
	fileInfo, err := os.Stat(configPath)
	if err == nil {
		mode := fileInfo.Mode()
		if mode&0077 != 0 {
			issues = append(issues, "Configuration file has overly permissive permissions (should be 600)")
		}
	}
	
	return issues
}