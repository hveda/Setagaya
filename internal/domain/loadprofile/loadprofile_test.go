package loadprofile_test

import (
	"errors"
	"testing"

	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
)

func validScenario(scenarioID int64, engines, concurrency int) loadprofile.Entry {
	return loadprofile.Entry{
		Name:        "p",
		ScenarioID:  scenarioID,
		Engines:     engines,
		Concurrency: concurrency,
		Rampup:      1,
		Duration:    60,
	}
}

func TestExecutionScenario_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ep      loadprofile.Entry
		wantErr error
	}{
		{"valid", validScenario(1, 2, 10), nil},
		{"no scenario id", loadprofile.Entry{Engines: 1, Concurrency: 1, Duration: 1}, loadprofile.ErrScenarioRequired},
		{"zero engines", loadprofile.Entry{ScenarioID: 1, Engines: 0, Concurrency: 1, Duration: 1}, loadprofile.ErrEnginesInvalid},
		{"zero concurrency", loadprofile.Entry{ScenarioID: 1, Engines: 1, Concurrency: 0, Duration: 1}, loadprofile.ErrConcurrencyInvalid},
		{"zero duration", loadprofile.Entry{ScenarioID: 1, Engines: 1, Concurrency: 1, Duration: 0}, loadprofile.ErrDurationInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.ep.Validate()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestExecutionExecution_Validate_And_TotalEngines(t *testing.T) {
	t.Parallel()

	ec := loadprofile.Profile{
		ExecutionID: 5,
		Tests: []loadprofile.Entry{
			validScenario(1, 2, 10),
			validScenario(2, 3, 10),
		},
	}
	if err := ec.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := ec.TotalEngines(); got != 5 {
		t.Fatalf("TotalEngines = %d, want 5", got)
	}
}

func TestExecutionExecution_Validate_Errors(t *testing.T) {
	t.Parallel()

	empty := loadprofile.Profile{ExecutionID: 5}
	if err := empty.Validate(); !errors.Is(err, loadprofile.ErrNoScenarios) {
		t.Fatalf("empty Validate = %v, want ErrNoScenarios", err)
	}

	bad := loadprofile.Profile{
		ExecutionID: 5,
		Tests:       []loadprofile.Entry{validScenario(1, 0, 1)}, // zero engines
	}
	if err := bad.Validate(); !errors.Is(err, loadprofile.ErrEnginesInvalid) {
		t.Fatalf("bad Validate = %v, want ErrEnginesInvalid", err)
	}
}
