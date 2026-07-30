package ports

import (
	"context"

	"github.com/heridotlife/honryu/internal/domain/scenario"
)

// ScenarioFiles is the set of files attached to a scenario: one optional JMX test file
// and any number of data files (e.g. CSV). Names only — storage keys/URLs are
// derived by the application from a naming convention.
type ScenarioFiles struct {
	TestFile string   // "" when no JMX has been uploaded
	Data     []string // non-JMX data files
}

// ScenarioRepository persists and retrieves Scenario aggregates and their file records.
type ScenarioRepository interface {
	CreateScenario(ctx context.Context, p scenario.Scenario) (int64, error)
	GetScenario(ctx context.Context, id int64) (scenario.Scenario, error)
	ListScenariosByProject(ctx context.Context, projectID int64) ([]scenario.Scenario, error)
	DeleteScenario(ctx context.Context, id int64) error

	// AddScenarioFile records a file for the scenario. isTest selects the JMX test-file
	// slot (one per scenario) vs a data file. Returns ErrFileExists on duplicates.
	AddScenarioFile(ctx context.Context, scenarioID int64, filename string, isTest bool) error
	// ScenarioFilesFor returns the scenario's recorded files.
	ScenarioFilesFor(ctx context.Context, scenarioID int64) (ScenarioFiles, error)
	// DeleteScenarioFile removes a file record for the scenario.
	DeleteScenarioFile(ctx context.Context, scenarioID int64, filename string, isTest bool) error

	// ScenarioInUse reports whether the scenario is referenced by any execution's
	// execution configuration.
	ScenarioInUse(ctx context.Context, scenarioID int64) (bool, error)
}
