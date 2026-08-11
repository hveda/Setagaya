// Package config loads and validates Honryu runtime configuration.
//
// Configuration is read from environment variables through an injected getenv
// function so it is fully testable without mutating the process environment,
// and there is no global config singleton — the loaded Config is passed
// explicitly to the components that need it.
package config

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// Config is the fully-resolved, validated runtime configuration.
type Config struct {
	HTTP       HTTPConfig
	DB         DBConfig
	Log        LogConfig
	Storage    StorageConfig
	Limits     LimitsConfig
	Cluster    ClusterConfig
	Auth       AuthConfig
	Scheduler  SchedulerConfig
	Calibrator CalibratorConfig
}

// SchedulerConfig configures cmd/scheduler's fire-due-occurrences loop.
type SchedulerConfig struct {
	// TickInterval is how often cmd/scheduler polls for a due occurrence to
	// claim and fire.
	TickInterval time.Duration
	// HorizonInterval is how often the recurring-schedule horizon-extension
	// pass runs -- much less frequent than TickInterval, since it only needs
	// to keep occurrences reserved out to the 7-day horizon, not react to a
	// fire time arriving.
	HorizonInterval time.Duration
}

// CalibratorConfig configures the engine-calibration advancement loop
// (calibrationapp.Service.AdvanceOne, ticked once per due job). cmd/calibrator
// always runs it as its own deployment; cmd/scheduler additionally hosts the
// same loop only when HostInScheduler is set, for installations that would
// rather not run a third process. Either way, AdvanceOne's own row-locked
// claim keeps more than one replica from double-driving the same job.
type CalibratorConfig struct {
	// TickInterval is how often the loop claims and advances one due
	// calibration job.
	TickInterval time.Duration
	// HostInScheduler is off by default: cmd/calibrator is the only host
	// unless explicitly opted in.
	HostInScheduler bool
}

// AuthConfig selects the authentication provider and toggles RBAC.
//
// Mode "none" authenticates every request as a fixed service-provider admin
// (local dev); "oidc" verifies bearer ID tokens against the configured issuer's
// JWKS. EnableRBAC turns on tenant-scoped authorization; when false the legacy
// owner-based checks apply. This replaces v2's hardcoded enableRBAC=true.
type AuthConfig struct {
	Mode       string // none|oidc
	EnableRBAC bool
	OIDC       OIDCConfig
}

// OIDCConfig configures the OIDC ID-token verifier (used when Mode is "oidc").
type OIDCConfig struct {
	Issuer   string
	Audience string
	JWKSURL  string
}

// ClusterConfig selects and configures the scheduler and executor used to run
// load tests.
//
// Scheduler "fake" keeps everything in-memory (local dev); "k8s" uses the
// in-cluster Kubernetes client. Engines run bzt in a pod and push their
// measurements back, so nothing here reaches into a cluster to collect results.
type ClusterConfig struct {
	Scheduler string // fake|k8s
	Namespace string
	// DefaultEngine is the engine an execution runs on when it names none.
	DefaultEngine taurus.Executor
	// EngineImages maps each supported engine to its pinned container image.
	// Engines do not share a runtime, so there is one image per engine rather
	// than one image for all of them.
	EngineImages map[taurus.Executor]string
	EnginePort   int
	Context      string // deployment context scoping running_scenario rows
	// IngestToken authenticates engine pods pushing measurements. Empty rejects
	// every push, so a deployment that has not configured one is closed rather
	// than open.
	IngestToken string
	// SidecarImage is the metrics sidecar image for the DEFAULT cluster (the
	// control plane's own, which has no registry row). A registered cluster
	// carries its own SidecarImage on its registry entry (Phase 8), since a
	// GKE cluster and an on-prem cluster need not share one image source.
	// Required when Scheduler is "k8s".
	SidecarImage string
	// IngestURL is where the DEFAULT cluster's sidecars push batches -- this
	// deployment's own ingest endpoint, reachable from inside that cluster. A
	// registered cluster carries its own IngestURL on its entry (Phase 8).
	// Required when Scheduler is "k8s".
	IngestURL string
	// AutoPurgeInterval is how often idle engines are swept; zero disables the
	// sweeper. AutoPurgeIdle is how long engines may sit idle before a sweep
	// purges them.
	AutoPurgeInterval time.Duration
	AutoPurgeIdle     time.Duration
	// CredentialKey is the hex-encoded (64 hex digits) app-held key that
	// encrypts BYOC cluster credentials at rest (AES-256-GCM). Empty disables
	// the cluster-registry management API (/api/clusters) -- a deployment that
	// does not register clusters need not configure it.
	CredentialKey string
}

// StorageConfig configures the object store used for uploaded artifacts.
//
// Driver "local" stores files under Root; "nexus" targets a Sonatype Nexus raw
// repository at BaseURL/repository/Repo, optionally with basic-auth credentials.
type StorageConfig struct {
	Driver   string // local|nexus
	Root     string // filesystem root for the local store
	BaseURL  string // public base URL: retrieval links (local) or Nexus server (nexus)
	Repo     string // nexus raw repository name
	Username string // nexus basic-auth username
	Password string // nexus basic-auth password
}

// LimitsConfig holds platform guardrails.
type LimitsConfig struct {
	MaxEnginesInExecution int
}

// HTTPConfig configures the API HTTP server.
type HTTPConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// Addr returns the listen address in ":port" form.
func (h HTTPConfig) Addr() string {
	return net.JoinHostPort("", strconv.Itoa(h.Port))
}

// DBConfig selects and configures the repository backend.
//
// Driver "fake" uses the in-memory repository (local dev / walking skeleton);
// "mysql" uses the MySQL adapter and requires DSN.
type DBConfig struct {
	Driver string
	DSN    string
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level  string // debug|info|warn|error
	Format string // json|text
}

const envPrefix = "HONRYU_"

// Load resolves configuration from the given getenv function, applying
// defaults for anything unset, then validates the result.
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	cfg := Config{
		HTTP:    HTTPConfig{Port: 8080, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second},
		DB:      DBConfig{Driver: "fake"},
		Log:     LogConfig{Level: "info", Format: "json"},
		Storage: StorageConfig{Driver: "local", Root: "storage-data"},
		Limits:  LimitsConfig{MaxEnginesInExecution: 500},
		Cluster: ClusterConfig{
			Scheduler: "fake",

			Namespace:     "default",
			DefaultEngine: taurus.ExecutorJMeter,
			EngineImages:  map[taurus.Executor]string{taurus.ExecutorJMeter: "honryu/engine-jmeter:5.6.3"},
			EnginePort:    8080,
			Context:       "default",
			AutoPurgeIdle: time.Hour,
		},
		Auth:       AuthConfig{Mode: "none"},
		Scheduler:  SchedulerConfig{TickInterval: 30 * time.Second, HorizonInterval: 24 * time.Hour},
		Calibrator: CalibratorConfig{TickInterval: 30 * time.Second},
	}

	var err error
	if cfg.HTTP.Port, err = intEnv(getenv, "HTTP_PORT", cfg.HTTP.Port); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.ReadTimeout, err = durEnv(getenv, "HTTP_READ_TIMEOUT", cfg.HTTP.ReadTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.WriteTimeout, err = durEnv(getenv, "HTTP_WRITE_TIMEOUT", cfg.HTTP.WriteTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTP.IdleTimeout, err = durEnv(getenv, "HTTP_IDLE_TIMEOUT", cfg.HTTP.IdleTimeout); err != nil {
		return Config{}, err
	}
	cfg.DB.Driver = strEnv(getenv, "DB_DRIVER", cfg.DB.Driver)
	cfg.DB.DSN = strEnv(getenv, "DB_DSN", cfg.DB.DSN)
	cfg.Log.Level = strEnv(getenv, "LOG_LEVEL", cfg.Log.Level)
	cfg.Log.Format = strEnv(getenv, "LOG_FORMAT", cfg.Log.Format)
	cfg.Storage.Driver = strEnv(getenv, "STORAGE_DRIVER", cfg.Storage.Driver)
	cfg.Storage.Root = strEnv(getenv, "STORAGE_ROOT", cfg.Storage.Root)
	cfg.Storage.BaseURL = strEnv(getenv, "STORAGE_BASE_URL", cfg.Storage.BaseURL)
	cfg.Storage.Repo = strEnv(getenv, "NEXUS_REPO", cfg.Storage.Repo)
	cfg.Storage.Username = strEnv(getenv, "NEXUS_USERNAME", cfg.Storage.Username)
	cfg.Storage.Password = strEnv(getenv, "NEXUS_PASSWORD", cfg.Storage.Password)
	if cfg.Limits.MaxEnginesInExecution, err = intEnv(getenv, "MAX_ENGINES", cfg.Limits.MaxEnginesInExecution); err != nil {
		return Config{}, err
	}
	cfg.Cluster.Scheduler = strEnv(getenv, "SCHEDULER", cfg.Cluster.Scheduler)
	cfg.Cluster.Namespace = strEnv(getenv, "K8S_NAMESPACE", cfg.Cluster.Namespace)
	cfg.Cluster.IngestToken = strEnv(getenv, "INGEST_TOKEN", cfg.Cluster.IngestToken)
	cfg.Cluster.SidecarImage = strEnv(getenv, "SIDECAR_IMAGE", cfg.Cluster.SidecarImage)
	cfg.Cluster.IngestURL = strEnv(getenv, "INGEST_URL", cfg.Cluster.IngestURL)
	cfg.Cluster.CredentialKey = strEnv(getenv, "CLUSTER_CREDENTIAL_KEY", cfg.Cluster.CredentialKey)
	cfg.Cluster.DefaultEngine = taurus.Executor(
		strEnv(getenv, "DEFAULT_ENGINE", string(cfg.Cluster.DefaultEngine)))
	if raw := strEnv(getenv, "ENGINE_IMAGES", ""); raw != "" {
		images, imgErr := ParseEngineImages(raw)
		if imgErr != nil {
			return Config{}, imgErr
		}
		cfg.Cluster.EngineImages = images
	}
	cfg.Cluster.Context = strEnv(getenv, "DEPLOY_CONTEXT", cfg.Cluster.Context)
	if cfg.Cluster.EnginePort, err = intEnv(getenv, "ENGINE_PORT", cfg.Cluster.EnginePort); err != nil {
		return Config{}, err
	}
	if cfg.Cluster.AutoPurgeInterval, err = durEnv(getenv, "AUTOPURGE_INTERVAL", cfg.Cluster.AutoPurgeInterval); err != nil {
		return Config{}, err
	}
	if cfg.Cluster.AutoPurgeIdle, err = durEnv(getenv, "AUTOPURGE_IDLE", cfg.Cluster.AutoPurgeIdle); err != nil {
		return Config{}, err
	}
	cfg.Auth.Mode = strEnv(getenv, "AUTH_MODE", cfg.Auth.Mode)
	if cfg.Auth.EnableRBAC, err = boolEnv(getenv, "ENABLE_RBAC", cfg.Auth.EnableRBAC); err != nil {
		return Config{}, err
	}
	cfg.Auth.OIDC.Issuer = strEnv(getenv, "OIDC_ISSUER", cfg.Auth.OIDC.Issuer)
	cfg.Auth.OIDC.Audience = strEnv(getenv, "OIDC_AUDIENCE", cfg.Auth.OIDC.Audience)
	cfg.Auth.OIDC.JWKSURL = strEnv(getenv, "OIDC_JWKS_URL", cfg.Auth.OIDC.JWKSURL)
	if cfg.Scheduler.TickInterval, err = durEnv(getenv, "SCHEDULER_TICK_INTERVAL", cfg.Scheduler.TickInterval); err != nil {
		return Config{}, err
	}
	if cfg.Scheduler.HorizonInterval, err = durEnv(getenv, "SCHEDULER_HORIZON_INTERVAL", cfg.Scheduler.HorizonInterval); err != nil {
		return Config{}, err
	}
	if cfg.Calibrator.TickInterval, err = durEnv(getenv, "CALIBRATOR_TICK_INTERVAL", cfg.Calibrator.TickInterval); err != nil {
		return Config{}, err
	}
	if cfg.Calibrator.HostInScheduler, err = boolEnv(getenv, "CALIBRATOR_HOST_IN_SCHEDULER", cfg.Calibrator.HostInScheduler); err != nil {
		return Config{}, err
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return fmt.Errorf("config: HTTP port %d out of range 1-65535", c.HTTP.Port)
	}
	if !oneOf(c.Log.Level, "debug", "info", "warn", "error") {
		return fmt.Errorf("config: invalid log level %q", c.Log.Level)
	}
	if !oneOf(c.Log.Format, "json", "text") {
		return fmt.Errorf("config: invalid log format %q", c.Log.Format)
	}
	if !oneOf(c.DB.Driver, "fake", "mysql") {
		return fmt.Errorf("config: invalid db driver %q", c.DB.Driver)
	}
	if c.DB.Driver == "mysql" && c.DB.DSN == "" {
		return fmt.Errorf("config: db driver mysql requires %sDB_DSN", envPrefix)
	}
	if c.Limits.MaxEnginesInExecution <= 0 {
		return fmt.Errorf("config: %sMAX_ENGINES must be positive", envPrefix)
	}
	if !oneOf(c.Cluster.Scheduler, "fake", "k8s") {
		return fmt.Errorf("config: invalid scheduler %q", c.Cluster.Scheduler)
	}
	if c.Cluster.Scheduler == "k8s" {
		if c.Cluster.SidecarImage == "" {
			return fmt.Errorf("config: scheduler k8s requires %sSIDECAR_IMAGE", envPrefix)
		}
		if c.Cluster.IngestURL == "" {
			return fmt.Errorf("config: scheduler k8s requires %sINGEST_URL", envPrefix)
		}
	}
	if err := c.Cluster.ValidateEngines(); err != nil {
		return err
	}
	if !oneOf(c.Storage.Driver, "local", "nexus") {
		return fmt.Errorf("config: invalid storage driver %q", c.Storage.Driver)
	}
	if c.Storage.Driver == "nexus" {
		if c.Storage.BaseURL == "" {
			return fmt.Errorf("config: storage driver nexus requires %sSTORAGE_BASE_URL", envPrefix)
		}
		if c.Storage.Repo == "" {
			return fmt.Errorf("config: storage driver nexus requires %sNEXUS_REPO", envPrefix)
		}
	}
	if c.Cluster.EnginePort < 1 || c.Cluster.EnginePort > 65535 {
		return fmt.Errorf("config: %sENGINE_PORT %d out of range 1-65535", envPrefix, c.Cluster.EnginePort)
	}
	if !oneOf(c.Auth.Mode, "none", "oidc") {
		return fmt.Errorf("config: invalid auth mode %q", c.Auth.Mode)
	}
	if c.Auth.Mode == "oidc" {
		if c.Auth.OIDC.Issuer == "" {
			return fmt.Errorf("config: auth mode oidc requires %sOIDC_ISSUER", envPrefix)
		}
		if c.Auth.OIDC.JWKSURL == "" {
			return fmt.Errorf("config: auth mode oidc requires %sOIDC_JWKS_URL", envPrefix)
		}
	}
	if c.Scheduler.TickInterval <= 0 {
		return fmt.Errorf("config: %sSCHEDULER_TICK_INTERVAL must be positive", envPrefix)
	}
	if c.Scheduler.HorizonInterval <= 0 {
		return fmt.Errorf("config: %sSCHEDULER_HORIZON_INTERVAL must be positive", envPrefix)
	}
	if c.Calibrator.TickInterval <= 0 {
		return fmt.Errorf("config: %sCALIBRATOR_TICK_INTERVAL must be positive", envPrefix)
	}
	return nil
}

func strEnv(getenv func(string) string, key, def string) string {
	if v := getenv(envPrefix + key); v != "" {
		return v
	}
	return def
}

func intEnv(getenv func(string) string, key string, def int) (int, error) {
	v := getenv(envPrefix + key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s%s must be an integer: %w", envPrefix, key, err)
	}
	return n, nil
}

func durEnv(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := getenv(envPrefix + key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s%s must be a duration: %w", envPrefix, key, err)
	}
	return d, nil
}

func boolEnv(getenv func(string) string, key string, def bool) (bool, error) {
	v := getenv(envPrefix + key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("config: %s%s must be a boolean: %w", envPrefix, key, err)
	}
	return b, nil
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
