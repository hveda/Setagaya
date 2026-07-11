package config

import (
	"testing"
	"time"
)

// envMap builds a getenv func from a map so tests never touch the real
// process environment (hermetic, parallel-safe).
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envMap(nil))
	if err != nil {
		t.Fatalf("Load with empty env: unexpected error: %v", err)
	}

	if cfg.HTTP.Port != 8080 {
		t.Errorf("HTTP.Port = %d, want 8080", cfg.HTTP.Port)
	}
	if cfg.HTTP.ReadTimeout != 15*time.Second {
		t.Errorf("HTTP.ReadTimeout = %s, want 15s", cfg.HTTP.ReadTimeout)
	}
	if cfg.HTTP.WriteTimeout != 15*time.Second {
		t.Errorf("HTTP.WriteTimeout = %s, want 15s", cfg.HTTP.WriteTimeout)
	}
	if cfg.HTTP.IdleTimeout != 60*time.Second {
		t.Errorf("HTTP.IdleTimeout = %s, want 60s", cfg.HTTP.IdleTimeout)
	}
	if cfg.DB.Driver != "fake" {
		t.Errorf("DB.Driver = %q, want %q", cfg.DB.Driver, "fake")
	}
	if cfg.Log.Level != "info" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "info")
	}
	if cfg.Log.Format != "json" {
		t.Errorf("Log.Format = %q, want %q", cfg.Log.Format, "json")
	}
	if cfg.Storage.Root != "storage-data" {
		t.Errorf("Storage.Root = %q, want storage-data", cfg.Storage.Root)
	}
	if cfg.Limits.MaxEnginesInCollection != 500 {
		t.Errorf("Limits.MaxEnginesInCollection = %d, want 500", cfg.Limits.MaxEnginesInCollection)
	}
	if cfg.Auth.Mode != "none" {
		t.Errorf("Auth.Mode = %q, want none", cfg.Auth.Mode)
	}
	if cfg.Auth.EnableRBAC {
		t.Error("Auth.EnableRBAC = true, want false by default (not hardcoded)")
	}
}

func TestLoad_AuthOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envMap(map[string]string{
		"SETAGAYA_AUTH_MODE":     "oidc",
		"SETAGAYA_ENABLE_RBAC":   "true",
		"SETAGAYA_OIDC_ISSUER":   "https://issuer.example",
		"SETAGAYA_OIDC_AUDIENCE": "setagaya",
		"SETAGAYA_OIDC_JWKS_URL": "https://issuer.example/jwks",
	}))
	if err != nil {
		t.Fatalf("Load auth overrides: %v", err)
	}
	if cfg.Auth.Mode != "oidc" || !cfg.Auth.EnableRBAC {
		t.Fatalf("Auth = %+v, want oidc + rbac enabled", cfg.Auth)
	}
	if cfg.Auth.OIDC.Issuer != "https://issuer.example" || cfg.Auth.OIDC.Audience != "setagaya" ||
		cfg.Auth.OIDC.JWKSURL != "https://issuer.example/jwks" {
		t.Fatalf("OIDC = %+v", cfg.Auth.OIDC)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envMap(map[string]string{
		"SETAGAYA_HTTP_PORT":          "9090",
		"SETAGAYA_HTTP_READ_TIMEOUT":  "5s",
		"SETAGAYA_HTTP_WRITE_TIMEOUT": "7s",
		"SETAGAYA_HTTP_IDLE_TIMEOUT":  "2m",
		"SETAGAYA_DB_DRIVER":          "mysql",
		"SETAGAYA_DB_DSN":             "user:pw@tcp(db:3306)/setagaya",
		"SETAGAYA_LOG_LEVEL":          "debug",
		"SETAGAYA_LOG_FORMAT":         "text",
		"SETAGAYA_STORAGE_ROOT":       "/data/setagaya",
		"SETAGAYA_STORAGE_BASE_URL":   "https://cdn.example.com",
		"SETAGAYA_MAX_ENGINES":        "42",
	}))
	if err != nil {
		t.Fatalf("Load with overrides: unexpected error: %v", err)
	}
	if cfg.Storage.Root != "/data/setagaya" || cfg.Storage.BaseURL != "https://cdn.example.com" {
		t.Errorf("Storage = %+v, want overridden root/baseURL", cfg.Storage)
	}
	if cfg.Limits.MaxEnginesInCollection != 42 {
		t.Errorf("MaxEnginesInCollection = %d, want 42", cfg.Limits.MaxEnginesInCollection)
	}

	if cfg.HTTP.Port != 9090 {
		t.Errorf("HTTP.Port = %d, want 9090", cfg.HTTP.Port)
	}
	if cfg.HTTP.ReadTimeout != 5*time.Second {
		t.Errorf("HTTP.ReadTimeout = %s, want 5s", cfg.HTTP.ReadTimeout)
	}
	if cfg.HTTP.IdleTimeout != 2*time.Minute {
		t.Errorf("HTTP.IdleTimeout = %s, want 2m", cfg.HTTP.IdleTimeout)
	}
	if cfg.DB.Driver != "mysql" {
		t.Errorf("DB.Driver = %q, want mysql", cfg.DB.Driver)
	}
	if cfg.DB.DSN != "user:pw@tcp(db:3306)/setagaya" {
		t.Errorf("DB.DSN = %q, want the mysql DSN", cfg.DB.DSN)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want debug", cfg.Log.Level)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want text", cfg.Log.Format)
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	t.Parallel()

	cases := map[string]map[string]string{
		"port not a number":   {"SETAGAYA_HTTP_PORT": "abc"},
		"port out of range":   {"SETAGAYA_HTTP_PORT": "70000"},
		"port zero":           {"SETAGAYA_HTTP_PORT": "0"},
		"bad read timeout":    {"SETAGAYA_HTTP_READ_TIMEOUT": "soon"},
		"bad write timeout":   {"SETAGAYA_HTTP_WRITE_TIMEOUT": "later"},
		"bad idle timeout":    {"SETAGAYA_HTTP_IDLE_TIMEOUT": "never"},
		"unknown log level":   {"SETAGAYA_LOG_LEVEL": "verbose"},
		"unknown log format":  {"SETAGAYA_LOG_FORMAT": "yaml"},
		"unknown db driver":   {"SETAGAYA_DB_DRIVER": "postgres"},
		"mysql without dsn":   {"SETAGAYA_DB_DRIVER": "mysql"},
		"bad max engines":     {"SETAGAYA_MAX_ENGINES": "-3"},
		"non-numeric engines": {"SETAGAYA_MAX_ENGINES": "lots"},
		"unknown scheduler":   {"SETAGAYA_SCHEDULER": "nomad"},
		"unknown executor":    {"SETAGAYA_EXECUTOR": "locust"},
		"bad engine port":     {"SETAGAYA_ENGINE_PORT": "99999"},
		"non-numeric port":    {"SETAGAYA_ENGINE_PORT": "eighty"},
		"bad purge interval":  {"SETAGAYA_AUTOPURGE_INTERVAL": "soon"},
		"bad purge idle":      {"SETAGAYA_AUTOPURGE_IDLE": "forever"},
		"unknown auth mode":   {"SETAGAYA_AUTH_MODE": "ldap"},
		"bad enable rbac":     {"SETAGAYA_ENABLE_RBAC": "maybe"},
		"oidc without issuer": {"SETAGAYA_AUTH_MODE": "oidc", "SETAGAYA_OIDC_JWKS_URL": "https://x/jwks"},
		"oidc without jwks":   {"SETAGAYA_AUTH_MODE": "oidc", "SETAGAYA_OIDC_ISSUER": "https://x"},
	}

	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Load(envMap(env)); err == nil {
				t.Fatalf("Load(%v): expected error, got nil", env)
			}
		})
	}
}

func TestLoad_NilGetenvUsesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load(nil): unexpected error: %v", err)
	}
	if cfg.HTTP.Port != 8080 || cfg.DB.Driver != "fake" || cfg.Log.Level != "info" {
		t.Fatalf("Load(nil) = %+v, want defaults", cfg)
	}
}

func TestConfig_HTTPAddr(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envMap(map[string]string{"SETAGAYA_HTTP_PORT": "1234"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := cfg.HTTP.Addr(), ":1234"; got != want {
		t.Errorf("HTTP.Addr() = %q, want %q", got, want)
	}
}
