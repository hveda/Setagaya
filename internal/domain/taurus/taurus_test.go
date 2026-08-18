package taurus_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/heridotlife/honryu/internal/domain/taurus"
)

func TestDuration_MarshalsInTaurusForm(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds", 30 * time.Second, "30s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 2 * time.Hour, "2h"},
		{"mixed rounds to seconds", 90 * time.Second, "90s"},
		{"sub-second rounds up to 1s", 400 * time.Millisecond, "1s"},
		{"zero", 0, "0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := yaml.Marshal(taurus.Duration(tc.d))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got := strings.TrimSpace(string(out)); got != tc.want {
				t.Errorf("Duration(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

func TestConfig_MarshalsTaurusShape(t *testing.T) {
	t.Parallel()

	cfg := taurus.Config{
		Execution: []taurus.Execution{{
			Executor:    "jmeter",
			Concurrency: 5,
			RampUp:      taurus.Duration(2 * time.Second),
			HoldFor:     taurus.Duration(12 * time.Second),
			Scenario:    "checkout",
		}},
		Scenarios: map[string]taurus.Scenario{
			"checkout": {
				DefaultAddress: "http://target.internal",
				Requests:       []taurus.Request{{URL: "/cart", Label: "cart", Method: "GET"}},
			},
		},
		Reporting: []taurus.Reporter{
			{Module: "passfail", Criteria: []string{"failures>5%, continue as failed"}},
			{Module: "final-stats"},
		},
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)

	// Keys must be Taurus's, not Go's.
	for _, want := range []string{
		"execution:", "scenarios:", "reporting:",
		"ramp-up: 2s", "hold-for: 12s", "concurrency: 5",
		"default-address: http://target.internal",
		"module: passfail",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("marshalled config missing %q\n---\n%s", want, got)
		}
	}
	// Empty optional fields must not appear at all: bzt rejects some empty values.
	for _, unwanted := range []string{"iterations:", "throughput:", "script:", "settings:", "modules:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("marshalled config should omit %q\n---\n%s", unwanted, got)
		}
	}
}

func TestConfig_RoundTrips(t *testing.T) {
	t.Parallel()

	in := taurus.Config{
		Execution: []taurus.Execution{{
			Executor: "k6", Concurrency: 3,
			RampUp: taurus.Duration(time.Second), HoldFor: taurus.Duration(time.Minute),
			Scenario: "s", Throughput: 100,
		}},
		Scenarios: map[string]taurus.Scenario{"s": {Script: "s.js"}},
	}
	data, err := yaml.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out taurus.Config
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Execution) != 1 {
		t.Fatalf("execution count = %d, want 1", len(out.Execution))
	}
	e := out.Execution[0]
	if e.Executor != "k6" || e.Concurrency != 3 || e.Scenario != "s" || e.Throughput != 100 {
		t.Errorf("execution round-trip = %+v", e)
	}
	if e.RampUp != taurus.Duration(time.Second) || e.HoldFor != taurus.Duration(time.Minute) {
		t.Errorf("durations round-trip = %v / %v", e.RampUp, e.HoldFor)
	}
	if out.Scenarios["s"].Script != "s.js" {
		t.Errorf("scenario round-trip = %+v", out.Scenarios["s"])
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	valid := func() taurus.Config {
		return taurus.Config{
			Execution: []taurus.Execution{{
				Executor: "jmeter", Concurrency: 1,
				HoldFor: taurus.Duration(time.Second), Scenario: "s",
			}},
			Scenarios: map[string]taurus.Scenario{"s": {Script: "s.jmx"}},
		}
	}

	cases := []struct {
		name    string
		mutate  func(*taurus.Config)
		wantErr error
	}{
		{"valid", func(*taurus.Config) {}, nil},
		{"no executions", func(c *taurus.Config) { c.Execution = nil }, taurus.ErrNoExecution},
		{"missing executor", func(c *taurus.Config) { c.Execution[0].Executor = "" }, taurus.ErrExecutorRequired},
		{"concurrency zero", func(c *taurus.Config) { c.Execution[0].Concurrency = 0 }, taurus.ErrConcurrencyInvalid},
		{"negative concurrency", func(c *taurus.Config) { c.Execution[0].Concurrency = -1 }, taurus.ErrConcurrencyInvalid},
		{"no scenario ref", func(c *taurus.Config) { c.Execution[0].Scenario = "" }, taurus.ErrScenarioRequired},
		{
			"scenario ref not defined",
			func(c *taurus.Config) { c.Execution[0].Scenario = "ghost" },
			taurus.ErrScenarioUndefined,
		},
		{
			"scenario has neither script nor requests",
			func(c *taurus.Config) { c.Scenarios["s"] = taurus.Scenario{} },
			taurus.ErrScenarioEmpty,
		},
		{
			"no duration bound",
			func(c *taurus.Config) { c.Execution[0].HoldFor = 0 },
			taurus.ErrDurationRequired,
		},
		{
			"iterations satisfy the duration bound",
			func(c *taurus.Config) { c.Execution[0].HoldFor = 0; c.Execution[0].Iterations = 10 },
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := valid()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// The scenario reference is the join between the two halves of a Taurus config
// and the most likely thing for a compiler bug to get wrong, so the error must
// name the offending reference.
func TestConfig_Validate_NamesUndefinedScenario(t *testing.T) {
	t.Parallel()

	cfg := taurus.Config{
		Execution: []taurus.Execution{{
			Executor: "jmeter", Concurrency: 1,
			HoldFor: taurus.Duration(time.Second), Scenario: "checkout",
		}},
		Scenarios: map[string]taurus.Scenario{"search": {Script: "s.jmx"}},
	}
	err := cfg.Validate()
	if !errors.Is(err, taurus.ErrScenarioUndefined) {
		t.Fatalf("Validate() = %v, want ErrScenarioUndefined", err)
	}
	if !strings.Contains(err.Error(), "checkout") {
		t.Errorf("error %q does not name the undefined scenario", err)
	}
}

func TestDuration_UnmarshalYAML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		yaml    string
		want    time.Duration
		wantErr bool
	}{
		{"seconds string", "30s", 30 * time.Second, false},
		{"minutes string", "5m", 5 * time.Minute, false},
		{"hours string", "2h", 2 * time.Hour, false},
		// bzt also accepts a bare number, meaning seconds.
		{"bare number is seconds", "45", 45 * time.Second, false},
		{"zero", "0s", 0, false},
		{"unparseable string", "\"soon\"", 0, true},
		{"wrong type", "[1, 2]", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var d taurus.Duration
			err := yaml.Unmarshal([]byte(tc.yaml), &d)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%q) = nil error, want failure", tc.yaml)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%q): %v", tc.yaml, err)
			}
			if time.Duration(d) != tc.want {
				t.Errorf("Unmarshal(%q) = %v, want %v", tc.yaml, time.Duration(d), tc.want)
			}
		})
	}
}

func TestExecutor_DeclarativeSupport(t *testing.T) {
	t.Parallel()

	// Established by running each engine under bzt in the Phase 0 spike and by
	// reading bzt 1.16's executor modules: k6 is script-only, the rest accept
	// the declarative `requests:` form.
	cases := []struct {
		exec        taurus.Executor
		declarative bool
		known       bool
	}{
		{taurus.ExecutorJMeter, true, true},
		{taurus.ExecutorGatling, true, true},
		{taurus.ExecutorLocust, true, true},
		{taurus.ExecutorApiritif, true, true},
		{taurus.ExecutorAB, true, true},
		{taurus.ExecutorSiege, true, true},
		{taurus.ExecutorK6, false, true},
		{taurus.Executor("nope"), false, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.exec), func(t *testing.T) {
			t.Parallel()
			if got := tc.exec.AcceptsDeclarativeRequests(); got != tc.declarative {
				t.Errorf("%s.AcceptsDeclarativeRequests() = %v, want %v", tc.exec, got, tc.declarative)
			}
			if got := tc.exec.Known(); got != tc.known {
				t.Errorf("%s.Known() = %v, want %v", tc.exec, got, tc.known)
			}
		})
	}
}

func TestDeclarativeExecutors_IsStableAndExcludesK6(t *testing.T) {
	t.Parallel()

	got := taurus.DeclarativeExecutors()
	if len(got) == 0 {
		t.Fatal("DeclarativeExecutors() is empty")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("DeclarativeExecutors() not sorted: %v", got)
		}
	}
	for _, e := range got {
		if e == taurus.ExecutorK6 {
			t.Error("k6 is script-only and must not be listed as declarative")
		}
	}
	// Callers must not be able to mutate the package's table.
	got[0] = taurus.Executor("mutated")
	if taurus.DeclarativeExecutors()[0] == taurus.Executor("mutated") {
		t.Error("DeclarativeExecutors() exposes its backing array")
	}
}
