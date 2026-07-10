package execution_test

import (
	"errors"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/domain/execution"
)

func validPlan(planID int64, engines, concurrency int) execution.ExecutionPlan {
	return execution.ExecutionPlan{
		Name:        "p",
		PlanID:      planID,
		Engines:     engines,
		Concurrency: concurrency,
		Rampup:      1,
		Duration:    60,
	}
}

func TestExecutionPlan_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ep      execution.ExecutionPlan
		wantErr error
	}{
		{"valid", validPlan(1, 2, 10), nil},
		{"no plan id", execution.ExecutionPlan{Engines: 1, Concurrency: 1, Duration: 1}, execution.ErrPlanRequired},
		{"zero engines", execution.ExecutionPlan{PlanID: 1, Engines: 0, Concurrency: 1, Duration: 1}, execution.ErrEnginesInvalid},
		{"zero concurrency", execution.ExecutionPlan{PlanID: 1, Engines: 1, Concurrency: 0, Duration: 1}, execution.ErrConcurrencyInvalid},
		{"zero duration", execution.ExecutionPlan{PlanID: 1, Engines: 1, Concurrency: 1, Duration: 0}, execution.ErrDurationInvalid},
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

func TestExecutionCollection_Validate_And_TotalEngines(t *testing.T) {
	t.Parallel()

	ec := execution.ExecutionCollection{
		CollectionID: 5,
		Tests: []execution.ExecutionPlan{
			validPlan(1, 2, 10),
			validPlan(2, 3, 10),
		},
	}
	if err := ec.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := ec.TotalEngines(); got != 5 {
		t.Fatalf("TotalEngines = %d, want 5", got)
	}
}

func TestExecutionCollection_Validate_Errors(t *testing.T) {
	t.Parallel()

	empty := execution.ExecutionCollection{CollectionID: 5}
	if err := empty.Validate(); !errors.Is(err, execution.ErrNoPlans) {
		t.Fatalf("empty Validate = %v, want ErrNoPlans", err)
	}

	bad := execution.ExecutionCollection{
		CollectionID: 5,
		Tests:        []execution.ExecutionPlan{validPlan(1, 0, 1)}, // zero engines
	}
	if err := bad.Validate(); !errors.Is(err, execution.ErrEnginesInvalid) {
		t.Fatalf("bad Validate = %v, want ErrEnginesInvalid", err)
	}
}
