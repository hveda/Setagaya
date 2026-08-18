package httpapi

import (
	"context"
	"net/http"
	"slices"
	"strconv"

	"github.com/heridotlife/honryu/internal/domain/rbac"
)

// rbacEnabled reports whether the RBAC authorization path is in force. When
// false, the legacy no-auth owner checks apply.
func (h *handlers) rbacEnabled() bool {
	return h.deps.Auth != nil && h.deps.Auth.Enabled()
}

// owns reports whether the current (no-auth) account may act on resources owned
// by owner. Used only on the legacy (RBAC-disabled) path.
func (h *handlers) owns(owner string) bool {
	return slices.Contains(h.deps.DefaultOwners, owner)
}

// authorize enforces access to a resource. In legacy mode it checks the no-auth
// owner set; in RBAC mode it consults the authenticated account's roles against
// the resource's tenant. It returns errForbidden when access is denied.
func (h *handlers) authorize(ctx context.Context, owner string, tenantID *int64, resource string, action rbac.Action) error {
	if !h.rbacEnabled() {
		if !h.owns(owner) {
			return errForbidden
		}
		return nil
	}
	dec := h.deps.Auth.Authorize(accountFrom(ctx), rbac.Request{Resource: resource, Action: action, TenantID: tenantID})
	if !dec.Allowed {
		return errForbidden
	}
	return nil
}

// authorizeProject loads a project and verifies the caller may mutate it. It
// returns ports.ErrNotFound if the project is absent, or errForbidden otherwise.
func (h *handlers) authorizeProject(ctx context.Context, projectID int64) error {
	p, err := h.deps.Projects.Get(ctx, projectID)
	if err != nil {
		return err
	}
	return h.authorize(ctx, p.Owner, p.TenantID, rbac.ResourceProject, rbac.ActionUpdate)
}

// pathInt parses a required int64 path parameter.
func pathInt(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}
