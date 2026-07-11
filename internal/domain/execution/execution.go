// Package execution holds the execution configuration for a Collection: which
// plans run, with how many engines, and at what concurrency. These types carry
// yaml tags for the uploaded collection config ("multi-test" wrapper) but the
// package itself performs no I/O and imports no serializer.
package execution

import (
	"errors"
	"fmt"
)

// Validation errors. Callers compare with errors.Is.
var (
	ErrPlanRequired       = errors.New("execution: a valid plan id is required")
	ErrEnginesInvalid     = errors.New("execution: engines must be greater than zero")
	ErrConcurrencyInvalid = errors.New("execution: concurrency must be greater than zero")
	ErrDurationInvalid    = errors.New("execution: duration must be greater than zero")
	ErrNoPlans            = errors.New("execution: at least one plan is required")
)

// ExecutionPlan is one plan's run configuration within a collection.
type ExecutionPlan struct {
	Name        string `yaml:"name" json:"name"`
	PlanID      int64  `yaml:"testid" json:"plan_id"`
	Concurrency int    `yaml:"concurrency" json:"concurrency"`
	Rampup      int    `yaml:"rampup" json:"rampup"`
	Engines     int    `yaml:"engines" json:"engines"`
	Duration    int    `yaml:"duration" json:"duration"`
	CSVSplit    bool   `yaml:"csv_split" json:"csv_split"`
}

// Validate checks a single execution plan's invariants.
func (ep ExecutionPlan) Validate() error {
	switch {
	case ep.PlanID <= 0:
		return ErrPlanRequired
	case ep.Engines <= 0:
		return ErrEnginesInvalid
	case ep.Concurrency <= 0:
		return ErrConcurrencyInvalid
	case ep.Duration <= 0:
		return ErrDurationInvalid
	}
	return nil
}

// ExecutionCollection is the full set of plans to run for a collection.
type ExecutionCollection struct {
	Name         string          `yaml:"name" json:"name"`
	ProjectID    int64           `yaml:"projectid" json:"project_id"`
	CollectionID int64           `yaml:"collectionid" json:"collection_id"`
	Tests        []ExecutionPlan `yaml:"tests" json:"tests"`
	CSVSplit     bool            `yaml:"csv_split" json:"csv_split"`
}

// Wrapper is the top-level shape of an uploaded collection config file.
type Wrapper struct {
	Content ExecutionCollection `yaml:"multi-test" json:"multi-test"`
}

// Validate ensures there is at least one plan and every plan is valid.
func (ec ExecutionCollection) Validate() error {
	if len(ec.Tests) == 0 {
		return ErrNoPlans
	}
	for i, ep := range ec.Tests {
		if err := ep.Validate(); err != nil {
			return fmt.Errorf("plan %d (id %d): %w", i, ep.PlanID, err)
		}
	}
	return nil
}

// TotalEngines is the sum of engines across all plans.
func (ec ExecutionCollection) TotalEngines() int {
	total := 0
	for _, ep := range ec.Tests {
		total += ep.Engines
	}
	return total
}
