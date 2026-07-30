package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/heridotlife/Setagaya/internal/app/collectionapp"
	"github.com/heridotlife/Setagaya/internal/app/lifecycleapp"
	"github.com/heridotlife/Setagaya/internal/app/planapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/tenantapp"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/domain/run"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/domain/tenant"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// errForbidden is returned by ownership checks; mapped to HTTP 403.
var errForbidden = errors.New("forbidden")

// badRequestErrors are client input/validation failures → HTTP 400.
var badRequestErrors = []error{
	project.ErrNameRequired, project.ErrNameTooLong, project.ErrOwnerRequired,
	project.ErrOwnerTooLong, project.ErrSIDInvalid, project.ErrSIDTooLong,
	scenario.ErrNameRequired, scenario.ErrNameTooLong, scenario.ErrProjectRequired,
	execution.ErrNameRequired, execution.ErrNameTooLong, execution.ErrProjectRequired,
	loadprofile.ErrPlanRequired, loadprofile.ErrEnginesInvalid, loadprofile.ErrConcurrencyInvalid,
	loadprofile.ErrDurationInvalid, loadprofile.ErrNoPlans,
	planapp.ErrInvalidFilename,
	collectionapp.ErrInvalidFilename, collectionapp.ErrCollectionMismatch,
	collectionapp.ErrPlanNotInProject, collectionapp.ErrEngineLimit,
	run.ErrNoPlans, lifecycleapp.ErrNoTestFile,
	tenant.ErrNameRequired, tenant.ErrNameTooLong, tenant.ErrNameInvalid,
	tenant.ErrDisplayNameRequired, tenant.ErrStatusInvalid,
	tenantapp.ErrUnknownRole, tenantapp.ErrGlobalRoleScoped,
}

// conflictErrors are state conflicts → HTTP 409.
var conflictErrors = []error{
	ports.ErrFileExists,
	planapp.ErrPlanInUse,
	projectapp.ErrProjectHasPlans, projectapp.ErrProjectHasCollections,
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
