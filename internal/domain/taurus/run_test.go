package taurus_test

import (
	"strings"
	"testing"

	"github.com/heridotlife/Setagaya/internal/domain/taurus"
)

func TestOutcomeFromExitCode(t *testing.T) {
	t.Parallel()

	// Established from bzt 1.16 source: NormalShutdown.get_rc() == 0,
	// ManualShutdown == 2, AutomatedShutdown == 3 (raised by modules/passfail),
	// and cli.py falls back to 1 for any other exception.
	cases := []struct {
		name string
		code int
		want taurus.Outcome
	}{
		{"success", 0, taurus.OutcomePassed},
		{"generic failure", 1, taurus.OutcomeError},
		{"interrupted", 2, taurus.OutcomeAborted},
		{"criteria tripped", 3, taurus.OutcomeFailed},
		{"unknown non-zero", 7, taurus.OutcomeError},
		{"sigterm via shell convention", 143, taurus.OutcomeError},
		{"negative", -1, taurus.OutcomeError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := taurus.OutcomeFromExitCode(tc.code); got != tc.want {
				t.Errorf("OutcomeFromExitCode(%d) = %q, want %q", tc.code, got, tc.want)
			}
		})
	}
}

// An aborted run is not a failed one. Honryu tears engines down for reasons that
// say nothing about the target -- the kill switch, a quota rejection, a campaign
// freeze -- and counting those as failures would corrupt a readiness verdict.
func TestOutcome_AbortIsNeitherPassNorFail(t *testing.T) {
	t.Parallel()

	aborted := taurus.OutcomeFromExitCode(2)
	if aborted == taurus.OutcomeFailed {
		t.Error("an interrupted run must not be reported as a failed one")
	}
	if aborted == taurus.OutcomeError {
		t.Error("an interrupted run must not be reported as an engine error")
	}
	if aborted.CountsTowardVerdict() {
		t.Error("an aborted run must not count toward a verdict")
	}
	for _, o := range []taurus.Outcome{taurus.OutcomePassed, taurus.OutcomeFailed} {
		if !o.CountsTowardVerdict() {
			t.Errorf("%q must count toward a verdict", o)
		}
	}
	if taurus.OutcomeError.CountsTowardVerdict() {
		t.Error("an engine error says nothing about the target and must not count")
	}
}

func TestCommand(t *testing.T) {
	t.Parallel()

	got := taurus.Command("/honryu/config/taurus.yml", "/honryu/artifacts")

	if len(got) == 0 || got[0] != "bzt" {
		t.Fatalf("Command()[0] = %v, want bzt", got)
	}
	joined := strings.Join(got, " ")
	for _, want := range []string{
		"/honryu/config/taurus.yml",
		"settings.artifacts-dir=/honryu/artifacts",
		// Engine pods must not depend on outbound internet; bzt phones home for
		// a version check otherwise.
		"settings.check-updates=false",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("Command() = %v, missing %q", got, want)
		}
	}
	// The config path must precede the overrides, or bzt reads them as configs.
	cfgAt, ovrAt := -1, -1
	for i, a := range got {
		if a == "/honryu/config/taurus.yml" {
			cfgAt = i
		}
		if a == "-o" && ovrAt == -1 {
			ovrAt = i
		}
	}
	if cfgAt == -1 || ovrAt == -1 || cfgAt > ovrAt {
		t.Errorf("Command() = %v, want the config path before any -o override", got)
	}
}

func TestCommand_OmitsEmptyArtifactsDir(t *testing.T) {
	t.Parallel()

	got := taurus.Command("/cfg.yml", "")
	for _, a := range got {
		if strings.Contains(a, "artifacts-dir") {
			t.Errorf("Command() = %v, should not set an empty artifacts-dir", got)
		}
	}
}
