package httpapi

import (
	"context"
	"net/http"

	"github.com/heridotlife/honryu/internal/app/adminapp"
	"github.com/heridotlife/honryu/internal/domain/rbac"
)

// adminExecutions lists the executions currently holding engines.
func (h *handlers) adminExecutions(w http.ResponseWriter, r *http.Request) {
	running, err := h.deps.Admin.RunningExecutions(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, running)
}

// adminNodes reports the cluster node pools.
func (h *handlers) adminNodes(w http.ResponseWriter, r *http.Request) {
	pools, err := h.deps.Admin.NodePools(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pools)
}

// abortExecutions is the kill-switch: tears down every in-flight execution
// matching scope+value (tenant, cluster, campaign, or execution_list) within
// a bounded time.
func (h *handlers) abortExecutions(w http.ResponseWriter, r *http.Request) {
	if err := h.authorizeKillSwitch(r.Context()); err != nil {
		respondError(w, err)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	scope := adminapp.Scope(r.PostForm.Get("scope"))
	value := r.PostForm.Get("value")

	aborted, err := h.deps.Admin.Abort(r.Context(), scope, value)
	if err != nil {
		respondError(w, err)
		return
	}
	h.audit(r.Context(), "admin.abort", string(scope), value)
	writeJSON(w, http.StatusOK, map[string]any{"aborted": aborted})
}

// authorizeKillSwitch permits the kill-switch. Under RBAC it requires a
// service-provider admin (rbac.ResourceSystem/ActionAdmin); in no-auth mode
// the operator has full access, matching every other admin route today.
func (h *handlers) authorizeKillSwitch(ctx context.Context) error {
	if !h.rbacEnabled() {
		return nil
	}
	dec := h.deps.Auth.Authorize(accountFrom(ctx), rbac.Request{Resource: rbac.ResourceSystem, Action: rbac.ActionAdmin})
	if !dec.Allowed {
		return errForbidden
	}
	return nil
}
