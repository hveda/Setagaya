package ports

import (
	"context"

	"github.com/hveda/Setagaya/v3/internal/domain/plan"
)

// PlanFiles is the set of files attached to a plan: one optional JMX test file
// and any number of data files (e.g. CSV). Names only — storage keys/URLs are
// derived by the application from a naming convention.
type PlanFiles struct {
	TestFile string   // "" when no JMX has been uploaded
	Data     []string // non-JMX data files
}

// PlanRepository persists and retrieves Plan aggregates and their file records.
type PlanRepository interface {
	CreatePlan(ctx context.Context, p plan.Plan) (int64, error)
	GetPlan(ctx context.Context, id int64) (plan.Plan, error)
	ListPlansByProject(ctx context.Context, projectID int64) ([]plan.Plan, error)
	DeletePlan(ctx context.Context, id int64) error

	// AddPlanFile records a file for the plan. isTest selects the JMX test-file
	// slot (one per plan) vs a data file. Returns ErrFileExists on duplicates.
	AddPlanFile(ctx context.Context, planID int64, filename string, isTest bool) error
	// PlanFilesFor returns the plan's recorded files.
	PlanFilesFor(ctx context.Context, planID int64) (PlanFiles, error)
	// DeletePlanFile removes a file record for the plan.
	DeletePlanFile(ctx context.Context, planID int64, filename string, isTest bool) error

	// PlanInUse reports whether the plan is referenced by any collection's
	// execution configuration.
	PlanInUse(ctx context.Context, planID int64) (bool, error)
}
