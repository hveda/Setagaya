package config

import (
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/taurus"
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
	if cfg.Storage.Driver != "local" {
		t.Errorf("Storage.Driver = %q, want local", cfg.Storage.Driver)
	}
	if cfg.Limits.MaxEnginesInExecution != 500 {
		t.Errorf("Limits.MaxEnginesInExecution = %d, want 500", cfg.Limits.MaxEnginesInExecution)
	}
	if cfg.Auth.Mode != "none" {
		t.Errorf("Auth.Mode = %q, want none", cfg.Auth.Mode)
	}
	if cfg.Auth.EnableRBAC {
		t.Error("Auth.EnableRBAC = true, want false by default (not hardcoded)")
	}
	if cfg.Scheduler.TickInterval != 30*time.Second {
		t.Errorf("Scheduler.TickInterval = %s, want 30s", cfg.Scheduler.TickInterval)
	}
	if cfg.Scheduler.HorizonInterval != 24*time.Hour {
		t.Errorf("Scheduler.HorizonInterval = %s, want 24h", cfg.Scheduler.HorizonInterval)
	}
	if cfg.Calibrator.TickInterval != 30*time.Second {
		t.Errorf("Calibrator.TickInterval = %s, want 30s", cfg.Calibrator.TickInterval)
	}
	if cfg.Calibrator.HostInScheduler {
		t.Error("Calibrator.HostInScheduler = true, want false by default")
	}
}

func TestLoad_AuthOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envMap(map[string]string{
		"HONRYU_AUTH_MODE":     "oidc",
		"HONRYU_ENABLE_RBAC":   "true",
		"HONRYU_OIDC_ISSUER":   "https://issuer.example",
		"HONRYU_OIDC_AUDIENCE": "honryu",
		"HONRYU_OIDC_JWKS_URL": "https://issuer.example/jwks",
	}))
	if err != nil {
		t.Fatalf("Load auth overrides: %v", err)
	}
	if cfg.Auth.Mode != "oidc" || !cfg.Auth.EnableRBAC {
		t.Fatalf("Auth = %+v, want oidc + rbac enabled", cfg.Auth)
	}
	if cfg.Auth.OIDC.Issuer != "https://issuer.example" || cfg.Auth.OIDC.Audience != "honryu" ||
		cfg.Auth.OIDC.JWKSURL != "https://issuer.example/jwks" {
		t.Fatalf("OIDC = %+v", cfg.Auth.OIDC)
	}
}

func TestLoad_StorageAndEngineOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envMap(map[string]string{
		"HONRYU_DEFAULT_ENGINE":   "k6",
		"HONRYU_ENGINE_IMAGES":    "jmeter=honryu/engine-jmeter:5.6.3,k6=honryu/engine-k6:1.0.0",
		"HONRYU_STORAGE_DRIVER":   "nexus",
		"HONRYU_STORAGE_BASE_URL": "https://nexus.example",
		"HONRYU_NEXUS_REPO":       "honryu-raw",
		"HONRYU_NEXUS_USERNAME":   "admin",
		"HONRYU_NEXUS_PASSWORD":   "s3cret",
	}))
	if err != nil {
		t.Fatalf("Load storage/executor overrides: %v", err)
	}
	if cfg.Cluster.DefaultEngine != taurus.ExecutorK6 {
		t.Fatalf("DefaultEngine = %q, want k6", cfg.Cluster.DefaultEngine)
	}
	if img, err := cfg.Cluster.ImageFor(taurus.ExecutorK6); err != nil || img != "honryu/engine-k6:1.0.0" {
		t.Fatalf("ImageFor(k6) = %q, %v", img, err)
	}
	if cfg.Storage.Driver != "nexus" || cfg.Storage.Repo != "honryu-raw" ||
		cfg.Storage.Username != "admin" || cfg.Storage.Password != "s3cret" {
		t.Fatalf("Storage = %+v", cfg.Storage)
	}
}

// The k8s scheduler adapter builds a sidecar container from these directly
// (internal/adapters/scheduler/k8s.Config.SidecarImage/IngestURL); an empty
// value would reach it as an invalid pod spec, so both are required once
// Scheduler is "k8s" (see TestLoad_ValidationErrors' k8s cases for the
// rejection side of this).
func TestLoad_K8sSchedulerRequiresSidecarAndIngestConfig(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envMap(map[string]string{
		"HONRYU_SCHEDULER":     "k8s",
		"HONRYU_SIDECAR_IMAGE": "registry.example/honryu-sidecar:1.0.0",
		"HONRYU_INGEST_URL":    "http://api.honryu.svc/api/ingest",
	}))
	if err != nil {
		t.Fatalf("Load with sidecar image and ingest url set: %v", err)
	}
	if cfg.Cluster.SidecarImage != "registry.example/honryu-sidecar:1.0.0" {
		t.Errorf("SidecarImage = %q", cfg.Cluster.SidecarImage)
	}
	if cfg.Cluster.IngestURL != "http://api.honryu.svc/api/ingest" {
		t.Errorf("IngestURL = %q", cfg.Cluster.IngestURL)
	}
}

func TestLoad_Overrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(envMap(map[string]string{
		"HONRYU_HTTP_PORT":                    "9090",
		"HONRYU_HTTP_READ_TIMEOUT":            "5s",
		"HONRYU_HTTP_WRITE_TIMEOUT":           "7s",
		"HONRYU_HTTP_IDLE_TIMEOUT":            "2m",
		"HONRYU_DB_DRIVER":                    "mysql",
		"HONRYU_DB_DSN":                       "user:pw@tcp(db:3306)/honryu",
		"HONRYU_LOG_LEVEL":                    "debug",
		"HONRYU_LOG_FORMAT":                   "text",
		"HONRYU_STORAGE_ROOT":                 "/data/honryu",
		"HONRYU_STORAGE_BASE_URL":             "https://cdn.example.com",
		"HONRYU_MAX_ENGINES":                  "42",
		"HONRYU_SCHEDULER_TICK_INTERVAL":      "5s",
		"HONRYU_SCHEDULER_HORIZON_INTERVAL":   "12h",
		"HONRYU_CALIBRATOR_TICK_INTERVAL":     "15s",
		"HONRYU_CALIBRATOR_HOST_IN_SCHEDULER": "true",
	}))
	if err != nil {
		t.Fatalf("Load with overrides: unexpected error: %v", err)
	}
	if cfg.Storage.Root != "/data/honryu" || cfg.Storage.BaseURL != "https://cdn.example.com" {
		t.Errorf("Storage = %+v, want overridden root/baseURL", cfg.Storage)
	}
	if cfg.Limits.MaxEnginesInExecution != 42 {
		t.Errorf("MaxEnginesInExecution = %d, want 42", cfg.Limits.MaxEnginesInExecution)
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
	if cfg.DB.DSN != "user:pw@tcp(db:3306)/honryu" {
		t.Errorf("DB.DSN = %q, want the mysql DSN", cfg.DB.DSN)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want debug", cfg.Log.Level)
	}
	if cfg.Log.Format != "text" {
		t.Errorf("Log.Format = %q, want text", cfg.Log.Format)
	}
	if cfg.Scheduler.TickInterval != 5*time.Second {
		t.Errorf("Scheduler.TickInterval = %s, want 5s", cfg.Scheduler.TickInterval)
	}
	if cfg.Scheduler.HorizonInterval != 12*time.Hour {
		t.Errorf("Scheduler.HorizonInterval = %s, want 12h", cfg.Scheduler.HorizonInterval)
	}
	if cfg.Calibrator.TickInterval != 15*time.Second {
		t.Errorf("Calibrator.TickInterval = %s, want 15s", cfg.Calibrator.TickInterval)
	}
	if !cfg.Calibrator.HostInScheduler {
		t.Error("Calibrator.HostInScheduler = false, want true")
	}
}

func TestLoad_ValidationErrors(t *testing.T) {
	t.Parallel()

	cases := map[string]map[string]string{
		"port not a number":   {"HONRYU_HTTP_PORT": "abc"},
		"port out of range":   {"HONRYU_HTTP_PORT": "70000"},
		"port zero":           {"HONRYU_HTTP_PORT": "0"},
		"bad read timeout":    {"HONRYU_HTTP_READ_TIMEOUT": "soon"},
		"bad write timeout":   {"HONRYU_HTTP_WRITE_TIMEOUT": "later"},
		"bad idle timeout":    {"HONRYU_HTTP_IDLE_TIMEOUT": "never"},
		"unknown log level":   {"HONRYU_LOG_LEVEL": "verbose"},
		"unknown log format":  {"HONRYU_LOG_FORMAT": "yaml"},
		"unknown db driver":   {"HONRYU_DB_DRIVER": "postgres"},
		"mysql without dsn":   {"HONRYU_DB_DRIVER": "mysql"},
		"bad max engines":     {"HONRYU_MAX_ENGINES": "-3"},
		"non-numeric engines": {"HONRYU_MAX_ENGINES": "lots"},
		"unknown scheduler":   {"HONRYU_SCHEDULER": "nomad"},
		"k8s scheduler without sidecar image": {
			"HONRYU_SCHEDULER":  "k8s",
			"HONRYU_INGEST_URL": "http://api.honryu.svc/api/ingest",
		},
		"k8s scheduler without ingest url": {
			"HONRYU_SCHEDULER":     "k8s",
			"HONRYU_SIDECAR_IMAGE": "registry.example/honryu-sidecar:1.0.0",
		},
		"unknown default engine": {"HONRYU_DEFAULT_ENGINE": "wat"},
		"default engine without an image": {
			"HONRYU_DEFAULT_ENGINE": "k6",
			"HONRYU_ENGINE_IMAGES":  "jmeter=honryu/engine-jmeter:5.6.3",
		},
		"untagged engine image":         {"HONRYU_ENGINE_IMAGES": "jmeter=honryu/engine-jmeter"},
		"bad engine port":               {"HONRYU_ENGINE_PORT": "99999"},
		"non-numeric port":              {"HONRYU_ENGINE_PORT": "eighty"},
		"bad purge interval":            {"HONRYU_AUTOPURGE_INTERVAL": "soon"},
		"bad purge idle":                {"HONRYU_AUTOPURGE_IDLE": "forever"},
		"unknown auth mode":             {"HONRYU_AUTH_MODE": "ldap"},
		"bad enable rbac":               {"HONRYU_ENABLE_RBAC": "maybe"},
		"oidc without issuer":           {"HONRYU_AUTH_MODE": "oidc", "HONRYU_OIDC_JWKS_URL": "https://x/jwks"},
		"oidc without jwks":             {"HONRYU_AUTH_MODE": "oidc", "HONRYU_OIDC_ISSUER": "https://x"},
		"unknown storage":               {"HONRYU_STORAGE_DRIVER": "s3"},
		"nexus without url":             {"HONRYU_STORAGE_DRIVER": "nexus", "HONRYU_NEXUS_REPO": "raw"},
		"nexus without repo":            {"HONRYU_STORAGE_DRIVER": "nexus", "HONRYU_STORAGE_BASE_URL": "https://x"},
		"bad tick interval":             {"HONRYU_SCHEDULER_TICK_INTERVAL": "never"},
		"zero tick interval":            {"HONRYU_SCHEDULER_TICK_INTERVAL": "0s"},
		"bad horizon interval":          {"HONRYU_SCHEDULER_HORIZON_INTERVAL": "never"},
		"zero horizon interval":         {"HONRYU_SCHEDULER_HORIZON_INTERVAL": "0s"},
		"bad calibrator tick interval":  {"HONRYU_CALIBRATOR_TICK_INTERVAL": "never"},
		"zero calibrator tick interval": {"HONRYU_CALIBRATOR_TICK_INTERVAL": "0s"},
		"bad calibrator host flag":      {"HONRYU_CALIBRATOR_HOST_IN_SCHEDULER": "maybe"},
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

	cfg, err := Load(envMap(map[string]string{"HONRYU_HTTP_PORT": "1234"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := cfg.HTTP.Addr(), ":1234"; got != want {
		t.Errorf("HTTP.Addr() = %q, want %q", got, want)
	}
}
