package model

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hveda/Setagaya/setagaya/config"
	_ "github.com/go-sql-driver/mysql"
)

func setupTestConfig() error {
	// Create a test config that doesn't require external dependencies
	testConfig := &config.SetagayaConfig{
		ProjectHome:     "test-project-home",
		UploadFileHelp:  "test-upload-help",
		DistributedMode: false,
		DevMode:         true,
		Context:         "test",
		DBConf: &config.MySQLConfig{
			Host:     "localhost",
			User:     "test",
			Password: "test",
			Database: "setagaya_test",
		},
		AuthConfig: &config.AuthConfig{
			AdminUsers: []string{"test-admin"},
			NoAuth:     true,
			SessionKey: "test-session-key",
			LdapConfig: &config.LdapConfig{
				BaseDN:         "dc=test,dc=local",
				SystemUser:     "test",
				SystemPassword: "test",
				LdapServer:     "localhost",
				LdapPort:       "389",
			},
		},
		ObjectStorage: &config.ObjectStorage{
			Provider: "local",
			Url:      "http://localhost:8080",
			Bucket:   "setagaya-test",
		},
		LogFormat: &config.LogFormat{
			Json: false,
		},
	}

	// Mock database connection for testing
	mockDB := &sql.DB{}
	testConfig.DBC = mockDB

	// Set the global config
	config.SC = testConfig
	return nil
}

func setupAndTeardown() error {
	// Skip database operations if no real DB connection
	if config.SC.DBC == nil {
		return nil
	}

	db := config.SC.DBC
	q, err := db.Prepare("delete from plan")
	if err != nil {
		return err
	}
	defer q.Close()
	_, err = q.Exec()
	if err != nil {
		return err
	}

	q, err = db.Prepare("delete from running_plan")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from collection")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from collection_plan")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from project")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from collection_run")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	q, err = db.Prepare("delete from collection_run_history")
	if err != nil {
		return err
	}
	_, err = q.Exec()
	if err != nil {
		return err
	}
	return nil
}

// SetupTestEnvironment initializes a test environment with mock config
func SetupTestEnvironment(t *testing.T) func() {
	// Store original config
	originalConfig := config.SC

	// Setup test config
	if err := setupTestConfig(); err != nil {
		t.Fatalf("Failed to setup test config: %v", err)
	}

	// Return cleanup function
	return func() {
		config.SC = originalConfig
	}
}

// CreateTestConfigFile creates a temporary config file for testing
func CreateTestConfigFile(t *testing.T) (string, func()) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	testConfig := map[string]interface{}{
		"bg_color":      "#fff",
		"project_home":  "test-project-home",
		"upload_file_help": "test-upload-help",
		"auth_config": map[string]interface{}{
			"admin_users":     []string{},
			"ldap_server":     "localhost",
			"ldap_port":       "389",
			"system_user":     "test",
			"system_password": "test",
			"base_dn":         "dc=test,dc=local",
			"no_auth":         true,
			"session_key":     "test-session-key",
		},
		"http_config": map[string]interface{}{
			"proxy": "",
		},
		"db": map[string]interface{}{
			"host":     "localhost",
			"user":     "test",
			"password": "test",
			"database": "setagaya_test",
		},
		"executors": map[string]interface{}{
			"cluster": map[string]interface{}{
				"on_demand":   false,
				"kind":        "k8s",
				"gc_duration": 15,
			},
			"in_cluster": false,
			"namespace":  "setagaya-executors-test",
			"jmeter": map[string]interface{}{
				"image": "setagaya:jmeter-test",
				"cpu":   "0.1",
				"mem":   "512Mi",
			},
			"pull_secret":                 "",
			"pull_policy":                 "IfNotPresent",
			"max_engines_in_collection":   10,
		},
		"ingress": map[string]interface{}{
			"image":      "setagaya:ingress-controller-test",
			"cpu":        "0.1",
			"lifespan":   "30m",
			"gc_period":  "30s",
		},
		"dashboard": map[string]interface{}{
			"url":               "http://localhost:3000",
			"run_dashboard":     "/d/test/setagaya",
			"engine_dashboard":  "/d/test/setagaya-engine-health",
		},
		"object_storage": map[string]interface{}{
			"provider": "local",
			"url":      "http://localhost:8080",
			"user":     "",
			"password": "",
			"bucket":   "setagaya-test",
		},
		"log_format": map[string]interface{}{
			"json": false,
		},
	}

	data, err := json.MarshalIndent(testConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Override the config file path
	originalPath := config.ConfigFilePath
	config.ConfigFilePath = configPath

	return configPath, func() {
		config.ConfigFilePath = originalPath
		os.Remove(configPath)
	}
}
