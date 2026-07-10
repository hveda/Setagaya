package httpapi

import (
	"context"
	"net/http"
	"slices"
	"strconv"
)

// owns reports whether the current (no-auth) account may act on resources owned
// by owner. Replaced by the auth adapter in a later phase.
func (h *handlers) owns(owner string) bool {
	return slices.Contains(h.deps.DefaultOwners, owner)
}

// authorizeProject loads a project and verifies the caller owns it. It returns
// ports.ErrNotFound if the project is absent, or errForbidden otherwise.
func (h *handlers) authorizeProject(ctx context.Context, projectID int64) error {
	p, err := h.deps.Projects.Get(ctx, projectID)
	if err != nil {
		return err
	}
	if !h.owns(p.Owner) {
		return errForbidden
	}
	return nil
}

// pathInt parses a required int64 path parameter.
func pathInt(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
