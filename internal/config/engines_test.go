package config_test

import (
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/config"
	"github.com/heridotlife/honryu/internal/domain/taurus"
)

func TestParseEngineImages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    map[taurus.Executor]string
		wantErr string
	}{
		{
			name: "one engine",
			in:   "jmeter=honryu/engine-jmeter:5.6.3",
			want: map[taurus.Executor]string{taurus.ExecutorJMeter: "honryu/engine-jmeter:5.6.3"},
		},
		{
			name: "several, with spaces",
			in:   "jmeter=a:1, k6=b:2 ,gatling=c:3",
			want: map[taurus.Executor]string{
				taurus.ExecutorJMeter:  "a:1",
				taurus.ExecutorK6:      "b:2",
				taurus.ExecutorGatling: "c:3",
			},
		},
		{"empty", "", map[taurus.Executor]string{}, ""},
		{"unknown engine", "wat=x:1", nil, "wat"},
		{"missing image", "jmeter=", nil, "jmeter"},
		{"malformed pair", "jmeter", nil, "jmeter"},
		// An image without a tag would drift silently as the registry moves.
		{"untagged image", "jmeter=honryu/engine-jmeter", nil, "tag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := config.ParseEngineImages(tc.in)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseEngineImages(%q) = nil error, want one mentioning %q", tc.in, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEngineImages(%q): %v", tc.in, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("got[%s] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// A default engine with no image would fail only when the first execution tried
// to run, in a pod, with a blank image reference. Startup is where that belongs.
func TestClusterConfig_ValidateEngines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		def     taurus.Executor
		images  map[taurus.Executor]string
		wantErr string
	}{
		{
			name:   "default has an image",
			def:    taurus.ExecutorJMeter,
			images: map[taurus.Executor]string{taurus.ExecutorJMeter: "a:1"},
		},
		{
			name:    "default engine is unknown",
			def:     taurus.Executor("wat"),
			images:  map[taurus.Executor]string{taurus.ExecutorJMeter: "a:1"},
			wantErr: "wat",
		},
		{
			name:    "default engine has no image",
			def:     taurus.ExecutorK6,
			images:  map[taurus.Executor]string{taurus.ExecutorJMeter: "a:1"},
			wantErr: "k6",
		},
		{
			name:    "no images configured at all",
			def:     taurus.ExecutorJMeter,
			images:  map[taurus.Executor]string{},
			wantErr: "no engine images",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := config.ClusterConfig{DefaultEngine: tc.def, EngineImages: tc.images}
			err := c.ValidateEngines()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateEngines() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("ValidateEngines() = %v, want an error mentioning %q", err, tc.wantErr)
			}
		})
	}
}

func TestClusterConfig_ImageFor(t *testing.T) {
	t.Parallel()

	c := config.ClusterConfig{
		DefaultEngine: taurus.ExecutorJMeter,
		EngineImages: map[taurus.Executor]string{
			taurus.ExecutorJMeter: "jm:1",
			taurus.ExecutorK6:     "k6:2",
		},
	}

	if got, err := c.ImageFor(taurus.ExecutorK6); err != nil || got != "k6:2" {
		t.Errorf("ImageFor(k6) = %q, %v", got, err)
	}
	// An empty selection means "whatever the operator configured as default".
	if got, err := c.ImageFor(""); err != nil || got != "jm:1" {
		t.Errorf("ImageFor(\"\") = %q, %v; want the default engine's image", got, err)
	}
	// Selecting an engine this deployment has no image for must say so by name,
	// not fall back to the default and run the wrong engine.
	got, err := c.ImageFor(taurus.ExecutorGatling)
	if err == nil {
		t.Fatalf("ImageFor(gatling) = %q, want an error", got)
	}
	if !strings.Contains(err.Error(), "gatling") {
		t.Errorf("error %q does not name the engine", err)
	}
}
