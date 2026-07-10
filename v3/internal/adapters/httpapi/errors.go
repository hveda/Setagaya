package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/hveda/Setagaya/v3/internal/app/collectionapp"
	"github.com/hveda/Setagaya/v3/internal/app/planapp"
	"github.com/hveda/Setagaya/v3/internal/app/projectapp"
	"github.com/hveda/Setagaya/v3/internal/domain/collection"
	"github.com/hveda/Setagaya/v3/internal/domain/execution"
	"github.com/hveda/Setagaya/v3/internal/domain/plan"
	"github.com/hveda/Setagaya/v3/internal/domain/project"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

// errForbidden is returned by ownership checks; mapped to HTTP 403.
var errForbidden = errors.New("forbidden")

// badRequestErrors are client input/validation failures → HTTP 400.
var badRequestErrors = []error{
	project.ErrNameRequired, project.ErrNameTooLong, project.ErrOwnerRequired,
	project.ErrOwnerTooLong, project.ErrSIDInvalid, project.ErrSIDTooLong,
	plan.ErrNameRequired, plan.ErrNameTooLong, plan.ErrProjectRequired,
	collection.ErrNameRequired, collection.ErrNameTooLong, collection.ErrProjectRequired,
	execution.ErrPlanRequired, execution.ErrEnginesInvalid, execution.ErrConcurrencyInvalid,
	execution.ErrDurationInvalid, execution.ErrNoPlans,
	planapp.ErrInvalidFilename,
	collectionapp.ErrInvalidFilename, collectionapp.ErrCollectionMismatch,
	collectionapp.ErrPlanNotInProject, collectionapp.ErrEngineLimit,
}

// conflictErrors are state conflicts → HTTP 409.
var conflictErrors = []error{
	ports.ErrFileExists,
	planapp.ErrPlanInUse,
	projectapp.ErrProjectHasPlans, projectapp.ErrProjectHasCollections,
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
