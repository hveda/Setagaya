package main

import (
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
