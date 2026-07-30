package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/app/tenantapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/domain/run"
	"github.com/heridotlife/honryu/internal/domain/scenario"
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
	scenarioapp.ErrInvalidFilename,
	executionapp.ErrInvalidFilename, executionapp.ErrExecutionMismatch,
	executionapp.ErrScenarioNotInProject, executionapp.ErrEngineLimit,
	run.ErrNoScenarios, lifecycleapp.ErrNoTestFile,
	tenant.ErrNameRequired, tenant.ErrNameTooLong, tenant.ErrNameInvalid,
	tenant.ErrDisplayNameRequired, tenant.ErrStatusInvalid,
	tenantapp.ErrUnknownRole, tenantapp.ErrGlobalRoleScoped,
}

// conflictErrors are state conflicts → HTTP 409.
var conflictErrors = []error{
	ports.ErrFileExists,
	scenarioapp.ErrScenarioInUse,
	projectapp.ErrProjectHasScenarios, projectapp.ErrProjectHasExecutions,
	run.ErrNotDeployed, run.ErrEnginesNotReady, run.ErrAlreadyRunning, run.ErrNotRunning,
	ports.ErrEnginesUnreachable, ports.ErrRunActive,
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
