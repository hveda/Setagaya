package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLabelMap(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    map[string]string
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"one pair", "probe=checkout-cart", map[string]string{"probe": "checkout-cart"}, false},
		{
			// Engine labels are frequently URLs, which contain "=" in queries;
			// only the last separator may split the pair.
			name: "url with a query string",
			in:   "http://svc/cart?ref=a=checkout-cart",
			want: map[string]string{"http://svc/cart?ref=a": "checkout-cart"},
		},
		{
			name: "json form for labels containing commas",
			in:   `{"GET /a,b":"ab","x":"y"}`,
			want: map[string]string{"GET /a,b": "ab", "x": "y"},
		},
		{"missing separator", "probe", nil, true},
		{"empty target", "probe=", nil, true},
		{"empty source", "=probe", nil, true},
		{"malformed json", `{"a":`, nil, true},
		{"empty segments are skipped", "a=b,,c=d", map[string]string{"a": "b", "c": "d"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseLabelMap(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseLabelMap(%q) = nil error, want one", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLabelMap(%q): %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("got[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestConfigFrom(t *testing.T) {
	t.Parallel()

	cfg, err := configFrom([]string{
		"-ingest-url", "http://control/ingest",
		"-stream", "/tmp/kpi.jsonl",
		"-execution-id", "7", "-scenario-id", "11", "-run-id", "3", "-shard-index", "2",
		"-flush-interval", "250ms",
		"-label-map", "http://svc/cart=checkout-cart",
	})
	if err != nil {
		t.Fatalf("configFrom: %v", err)
	}
	if cfg.IngestURL != "http://control/ingest" || cfg.StreamPath != "/tmp/kpi.jsonl" {
		t.Errorf("paths = %+v", cfg)
	}
	id := cfg.Identity
	if id.ExecutionID != 7 || id.ScenarioID != 11 || id.RunID != 3 || id.ShardIndex != 2 {
		t.Errorf("identity = %+v", id)
	}
	if cfg.FlushInterval != 250*time.Millisecond {
		t.Errorf("FlushInterval = %v", cfg.FlushInterval)
	}
	if cfg.LabelMap["http://svc/cart"] != "checkout-cart" {
		t.Errorf("LabelMap = %+v", cfg.LabelMap)
	}
}

// Without somewhere to push, the sidecar would collect measurements and drop
// them, which is worse than refusing to start.
func TestConfigFrom_RequiresAnIngestURL(t *testing.T) {
	t.Parallel()

	if _, err := configFrom([]string{"-execution-id", "1"}); err == nil {
		t.Fatal("configFrom without -ingest-url succeeded")
	}
}

func TestConfigFrom_RejectsBadArguments(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"-ingest-url", "http://x", "-label-map", "no-separator"},
		{"-ingest-url", "http://x", "-execution-id", "not-a-number"},
		{"-ingest-url", "http://x", "-flush-interval", "soon"},
	}
	for _, args := range cases {
		if _, err := configFrom(args); err == nil {
			t.Errorf("configFrom(%v) succeeded, want an error", args)
		}
	}
}

// The token is a credential, so it comes from the environment rather than a
// command line other processes can read.
func TestConfigFrom_TakesTheTokenFromTheEnvironment(t *testing.T) {
	t.Setenv("HONRYU_INGEST_TOKEN", "s3cret")

	cfg, err := configFrom([]string{"-ingest-url", "http://x"})
	if err != nil {
		t.Fatalf("configFrom: %v", err)
	}
	if cfg.Token != "s3cret" {
		t.Errorf("Token = %q, want it read from the environment", cfg.Token)
	}
}

func TestRun_RejectsBadConfiguration(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"-ingest-url", "http://x", "-label-map", "no-separator"},
		{"-execution-id", "not-a-number"},
	}
	for _, args := range cases {
		if err := run(args); err == nil {
			t.Errorf("run(%v) succeeded, want a configuration error", args)
		}
	}
}

// End to end through run(): the engine already wrote its exit code, so the
// sidecar pushes its final batch to the ingest and finishes cleanly.
func TestRun_FinishesWhenTheEngineWroteItsExitCode(t *testing.T) {
	t.Parallel()

	var pushes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pushes++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	stream := filepath.Join(dir, "stream.jsonl")
	if err := os.WriteFile(stream, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stream: %v", err)
	}
	exitCode := filepath.Join(dir, "exit-code")
	if err := os.WriteFile(exitCode, []byte("0"), 0o644); err != nil {
		t.Fatalf("write exit code: %v", err)
	}

	if err := run([]string{
		"-ingest-url", srv.URL,
		"-stream", stream,
		"-exit-code", exitCode,
		"-flush-interval", "10ms",
	}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if pushes == 0 {
		t.Fatal("the sidecar finished without ever pushing a batch")
	}
}

// A final push the ingest rejects surfaces as run's error: measurements not
// accepted by the control plane are lost, so failing loudly is correct.
func TestRun_SurfacesIngestFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stream := filepath.Join(dir, "stream.jsonl")
	if err := os.WriteFile(stream, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stream: %v", err)
	}
	exitCode := filepath.Join(dir, "exit-code")
	if err := os.WriteFile(exitCode, []byte("0"), 0o644); err != nil {
		t.Fatalf("write exit code: %v", err)
	}

	if err := run([]string{
		"-ingest-url", "http://127.0.0.1:1/ingest",
		"-stream", stream,
		"-exit-code", exitCode,
		"-flush-interval", "10ms",
	}); err == nil {
		t.Fatal("run with an unreachable ingest: expected error, got nil")
	}
}
