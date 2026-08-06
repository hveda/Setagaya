package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/app/tenantapp"
	"github.com/heridotlife/honryu/internal/domain/compile"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/jmx"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/run"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/schedule"
	"github.com/heridotlife/honryu/internal/domain/tenant"
	"github.com/heridotlife/honryu/internal/ports"
)

// errForbidden is returned by ownership checks; mapped to HTTP 403.
var errForbidden = errors.New("forbidden")

// badRequestErrors are client input/validation failures → HTTP 400.
var badRequestErrors = []error{
	project.ErrNameRequired, project.ErrNameTooLong, project.ErrOwnerRequired,
	project.ErrOwnerTooLong, project.ErrSIDInvalid, project.ErrSIDTooLong,
	scenario.ErrNameRequired, scenario.ErrNameTooLong, scenario.ErrProjectRequired,
	execution.ErrNameRequired, execution.ErrNameTooLong, execution.ErrProjectRequired,
	loadprofile.ErrScenarioRequired, loadprofile.ErrEnginesInvalid, loadprofile.ErrConcurrencyInvalid,
	loadprofile.ErrDurationInvalid, loadprofile.ErrNoScenarios,
	scenarioapp.ErrInvalidFilename, scenarioapp.ErrRequestsInvalid,
	// An unusable JMeter plan is the caller's file, not a server fault: the
	// import must say which of the three ways it was unusable.
	jmx.ErrMalformed, jmx.ErrNotJMX, jmx.ErrNoTestPlan,
	executionapp.ErrInvalidFilename, executionapp.ErrExecutionMismatch,
	executionapp.ErrScenarioNotInProject, executionapp.ErrEngineLimit,
	run.ErrNoScenarios, lifecycleapp.ErrNoTestFile,
	// A portable scenario deployed with no requests uploaded yet is a
	// configuration gap on the caller's side, the same as a native scenario
	// with no script (ErrNoTestFile above) -- not a server fault.
	compile.ErrRequestsRequired,
	tenant.ErrNameRequired, tenant.ErrNameTooLong, tenant.ErrNameInvalid,
	tenant.ErrDisplayNameRequired, tenant.ErrStatusInvalid,
	tenantapp.ErrCeilingInvalid,
	// An unsequenced batch is a sidecar contract violation, not a transient
	// failure -- retrying it would never succeed, so it must not read as one.
	ports.ErrUnsequencedBatch,
	tenantapp.ErrUnknownRole, tenantapp.ErrGlobalRoleScoped,
	schedule.ErrExecutionRequired, schedule.ErrKindInvalid, schedule.ErrFireAtRequired,
	schedule.ErrRecurrenceRequired, schedule.ErrRecurrenceInvalid, schedule.ErrWindowInvalid,
}

// conflictErrors are state conflicts → HTTP 409.
var conflictErrors = []error{
	ports.ErrFileExists,
	scenarioapp.ErrScenarioInUse, scenarioapp.ErrScenarioNotPortable,
	projectapp.ErrProjectHasScenarios, projectapp.ErrProjectHasExecutions,
	run.ErrNotDeployed, run.ErrEnginesNotReady, run.ErrAlreadyRunning, run.ErrNotRunning,
	ports.ErrEnginesUnreachable, ports.ErrRunActive,
	// A pod pushing to an execution that is not running, or pushing for a run
	// that has ended, is a state conflict rather than a server fault -- and a
	// pod that outlived its run will do exactly this on every retry.
	metricsapp.ErrNoActiveRun, metricsapp.ErrStaleRun,
}

// respondError maps an application/domain error onto an HTTP status.
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound), errors.Is(err, ports.ErrObjectNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	case matchesAny(err, conflictErrors):
		writeError(w, http.StatusConflict, err.Error())
	case matchesAny(err, badRequestErrors):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		slog.Error("httpapi: internal error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func matchesAny(err error, sentinels []error) bool {
	for _, s := range sentinels {
		if errors.Is(err, s) {
			return true
		}
	}
	return false
}
