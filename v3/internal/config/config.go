// Package config loads and validates Setagaya v3 runtime configuration.
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
)

// Config is the fully-resolved, validated runtime configuration.
type Config struct {
	HTTP    HTTPConfig
	DB      DBConfig
	Log     LogConfig
	Storage StorageConfig
	Limits  LimitsConfig
	Cluster ClusterConfig
}

// ClusterConfig selects and configures the scheduler and executor used to run
// load tests.
//
// Scheduler "fake" keeps everything in-memory (local dev); "k8s" uses the
// in-cluster Kubernetes client. Executor "fake" records triggers; "jmeter"
// drives the JMeter agent over HTTP.
type ClusterConfig struct {
	Scheduler   string // fake|k8s
	Executor    string // fake|jmeter
	Namespace   string
	EngineImage string
	EnginePort  int
	Context     string // deployment context scoping running_plan rows
}

// StorageConfig configures the object store used for uploaded artifacts.
type StorageConfig struct {
	Root    string // filesystem root for the local store
	BaseURL string // optional public base URL for retrieval links
}

// LimitsConfig holds platform guardrails.
type LimitsConfig struct {
	MaxEnginesInCollection int
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

const envPrefix = "SETAGAYA_"

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
		Storage: StorageConfig{Root: "storage-data"},
		Limits:  LimitsConfig{MaxEnginesInCollection: 500},
		Cluster: ClusterConfig{
			Scheduler:   "fake",
			Executor:    "fake",
			Namespace:   "default",
			EngineImage: "setagaya/jmeter:latest",
			EnginePort:  8080,
			Context:     "default",
		},
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
	cfg.Storage.Root = strEnv(getenv, "STORAGE_ROOT", cfg.Storage.Root)
	cfg.Storage.BaseURL = strEnv(getenv, "STORAGE_BASE_URL", cfg.Storage.BaseURL)
	if cfg.Limits.MaxEnginesInCollection, err = intEnv(getenv, "MAX_ENGINES", cfg.Limits.MaxEnginesInCollection); err != nil {
		return Config{}, err
	}
	cfg.Cluster.Scheduler = strEnv(getenv, "SCHEDULER", cfg.Cluster.Scheduler)
	cfg.Cluster.Executor = strEnv(getenv, "EXECUTOR", cfg.Cluster.Executor)
	cfg.Cluster.Namespace = strEnv(getenv, "K8S_NAMESPACE", cfg.Cluster.Namespace)
	cfg.Cluster.EngineImage = strEnv(getenv, "ENGINE_IMAGE", cfg.Cluster.EngineImage)
	cfg.Cluster.Context = strEnv(getenv, "DEPLOY_CONTEXT", cfg.Cluster.Context)
	if cfg.Cluster.EnginePort, err = intEnv(getenv, "ENGINE_PORT", cfg.Cluster.EnginePort); err != nil {
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
	if c.Limits.MaxEnginesInCollection <= 0 {
		return fmt.Errorf("config: %sMAX_ENGINES must be positive", envPrefix)
	}
	if !oneOf(c.Cluster.Scheduler, "fake", "k8s") {
		return fmt.Errorf("config: invalid scheduler %q", c.Cluster.Scheduler)
	}
	if !oneOf(c.Cluster.Executor, "fake", "jmeter") {
		return fmt.Errorf("config: invalid executor %q", c.Cluster.Executor)
	}
	if c.Cluster.EnginePort < 1 || c.Cluster.EnginePort > 65535 {
		return fmt.Errorf("config: %sENGINE_PORT %d out of range 1-65535", envPrefix, c.Cluster.EnginePort)
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

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
