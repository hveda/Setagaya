// Package loadprofile holds the load configuration for a Execution: which
// scenarios run, with how many engines, and at what concurrency. An Entry is the
// unit that maps onto a Taurus execution block. These types carry yaml tags for
// the uploaded execution config ("multi-test" wrapper) but the package itself
// performs no I/O and imports no serializer.
package loadprofile

import (
	"errors"
	"fmt"
)

// Validation errors. Callers compare with errors.Is.
var (
	ErrScenarioRequired   = errors.New("loadprofile: a valid scenario id is required")
	ErrEnginesInvalid     = errors.New("loadprofile: engines must be greater than zero")
	ErrConcurrencyInvalid = errors.New("loadprofile: concurrency must be greater than zero")
	ErrDurationInvalid    = errors.New("loadprofile: duration must be greater than zero")
	ErrThroughputInvalid  = errors.New("loadprofile: throughput cannot be negative")
	ErrNoScenarios        = errors.New("loadprofile: at least one scenario is required")
)

// Entry is one scenario's load configuration within an execution; it maps onto
// a single Taurus execution block.
type Entry struct {
	Name        string `yaml:"name" json:"name"`
	ScenarioID  int64  `yaml:"testid" json:"scenario_id"`
	Concurrency int    `yaml:"concurrency" json:"concurrency"`
	Rampup      int    `yaml:"rampup" json:"rampup"`
	Engines     int    `yaml:"engines" json:"engines"`
	// Throughput is the target request rate for the entry, shared across its
	// engines. Zero means unlimited, which is what Taurus assumes when the key
	// is absent.
	Throughput int  `yaml:"throughput,omitempty" json:"throughput,omitempty"`
	Duration   int  `yaml:"duration" json:"duration"`
	CSVSplit   bool `yaml:"csv_split" json:"csv_split"`
}

// Validate checks a single entry's invariants.
func (ep Entry) Validate() error {
	switch {
	case ep.ScenarioID <= 0:
		return ErrScenarioRequired
	case ep.Engines <= 0:
		return ErrEnginesInvalid
	case ep.Concurrency <= 0:
		return ErrConcurrencyInvalid
	case ep.Duration <= 0:
		return ErrDurationInvalid
	case ep.Throughput < 0:
		return ErrThroughputInvalid
	}
	return nil
}

// Profile is the full set of load entries to run for an execution.
type Profile struct {
	Name        string  `yaml:"name" json:"name"`
	ProjectID   int64   `yaml:"projectid" json:"project_id"`
	ExecutionID int64   `yaml:"collectionid" json:"execution_id"`
	Tests       []Entry `yaml:"tests" json:"tests"`
	CSVSplit    bool    `yaml:"csv_split" json:"csv_split"`
}

// Wrapper is the top-level shape of an uploaded execution config file.
type Wrapper struct {
	Content Profile `yaml:"multi-test" json:"multi-test"`
}

// Validate ensures there is at least one scenario and every scenario is valid.
func (ec Profile) Validate() error {
	if len(ec.Tests) == 0 {
		return ErrNoScenarios
	}
	for i, ep := range ec.Tests {
		if err := ep.Validate(); err != nil {
			return fmt.Errorf("scenario %d (id %d): %w", i, ep.ScenarioID, err)
		}
	}
	return nil
}

// TotalEngines is the sum of engines across all scenarios.
func (ec Profile) TotalEngines() int {
	total := 0
	for _, ep := range ec.Tests {
		total += ep.Engines
	}
	return total
}

// LongestDurationSeconds is the longest scenario's ramp-up plus hold time --
// how long the profile can actually occupy engines, matching how
// compile.Taurus turns those same fields into a shard's actual run time
// (RampUp + HoldFor). A quota reservation for the whole profile should cover
// exactly this long, not an approximation.
func (ec Profile) LongestDurationSeconds() int {
	longest := 0
	for _, ep := range ec.Tests {
		if d := ep.Rampup + ep.Duration; d > longest {
			longest = d
		}
	}
	return longest
}
