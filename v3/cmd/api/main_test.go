package main

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/hveda/Setagaya/v3/internal/config"
)

func TestNewProjectRepository_Fake(t *testing.T) {
	t.Parallel()
	repo, err := newRepository(config.DBConfig{Driver: "fake"})
	if err != nil {
		t.Fatalf("newRepository(fake): %v", err)
	}
	if repo == nil {
		t.Fatal("newRepository(fake) returned nil repo")
	}
}

func TestNewProjectRepository_Unsupported(t *testing.T) {
	t.Parallel()
	if _, err := newRepository(config.DBConfig{Driver: "postgres"}); err == nil {
		t.Fatal("newRepository(postgres): expected error, got nil")
	}
}

func TestNewProjectRepository_MySQL_Unreachable(t *testing.T) {
	t.Parallel()
	// Nothing listens on port 1, so the ping fails fast: covers the mysql
	// open-ok / ping-error wiring branch without needing a container.
	_, err := newRepository(config.DBConfig{
		Driver: "mysql",
		DSN:    "setagaya:secret@tcp(127.0.0.1:1)/setagaya?parseTime=true",
	})
	if err == nil {
		t.Fatal("newRepository(mysql, unreachable): expected error, got nil")
	}
}

func TestSetupLogging_AllVariants(t *testing.T) {
	// Not parallel: mutates the global slog default logger.
	for _, c := range []config.LogConfig{
		{Level: "debug", Format: "text"},
		{Level: "info", Format: "json"},
		{Level: "warn", Format: "json"},
		{Level: "error", Format: "text"},
		{Level: "unknown", Format: "unknown"}, // exercises the default arms
	} {
		setupLogging(c)
	}
}

func TestRun_ConfigError(t *testing.T) {
	t.Parallel()
	env := map[string]string{"SETAGAYA_HTTP_PORT": "not-a-number"}
	err := run(context.Background(), func(k string) string { return env[k] })
	if err == nil {
		t.Fatal("run with invalid config: expected error, got nil")
	}
}

func TestRealMain_ConfigError(t *testing.T) {
	// t.Setenv marks the test non-parallel and restores the env afterwards.
	t.Setenv("SETAGAYA_HTTP_PORT", "not-a-number")
	if err := realMain(); err == nil {
		t.Fatal("realMain with invalid config: expected error, got nil")
	}
}

func TestRun_ServesAndShutsDownCleanly(t *testing.T) {
	t.Parallel()

	port := freePort(t)
	env := map[string]string{
		"SETAGAYA_HTTP_PORT":  strconv.Itoa(port),
		"SETAGAYA_DB_DRIVER":  "fake",
		"SETAGAYA_LOG_FORMAT": "text",
	}
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- run(ctx, func(k string) string { return env[k] }) }()

	// Wait until the server answers, then trigger graceful shutdown.
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	waitReady(t, base+"/healthz")

	resp, err := http.Get(base + "/api/projects")
	if err != nil {
		t.Fatalf("GET /api/projects: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/projects status = %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error on shutdown: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run did not shut down within 15s")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitReady(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server at %s not ready in time", url)
}
