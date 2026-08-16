package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
	"github.com/heridotlife/honryu/internal/domain/rbac"
)

// ClusterService is the cluster-registry surface the HTTP layer needs
// (clusterapp.Service satisfies it). Registration comes in two forms: operator
// (an existing home-cluster Secret named by secret_ref) and BYOC (an uploaded
// self-contained kubeconfig).
type ClusterService interface {
	RegisterOperator(ctx context.Context, entry clusterregistry.Cluster) (clusterregistry.Cluster, error)
	RegisterBYOC(ctx context.Context, entry clusterregistry.Cluster, kubeconfig []byte) (clusterregistry.Cluster, error)
	Get(ctx context.Context, name string) (clusterregistry.Cluster, error)
	List(ctx context.Context) ([]clusterregistry.Cluster, error)
	Update(ctx context.Context, name, ingestURL, sidecarImage, namespace string) (clusterregistry.Cluster, error)
	Delete(ctx context.Context, name string) error
}

// clusterResponse is a registered cluster as returned by the API. The
// credential (Secret contents / encrypted kubeconfig) is never surfaced --
// secret_ref names it, that is all.
type clusterResponse struct {
	Name         string    `json:"name"`
	APIURL       string    `json:"api_url"`
	IngestURL    string    `json:"ingest_url"`
	SidecarImage string    `json:"sidecar_image"`
	Namespace    string    `json:"namespace"`
	SecretRef    string    `json:"secret_ref"`
	Origin       string    `json:"origin"`
	CreatedBy    string    `json:"created_by,omitempty"`
	CreatedTime  time.Time `json:"created_time"`
}

func toClusterResponse(c clusterregistry.Cluster) clusterResponse {
	return clusterResponse{
		Name:         c.Name,
		APIURL:       c.APIURL,
		IngestURL:    c.IngestURL,
		SidecarImage: c.SidecarImage,
		Namespace:    c.Namespace,
		SecretRef:    c.SecretRef,
		Origin:       string(c.Origin),
		CreatedBy:    c.CreatedBy,
		CreatedTime:  c.CreatedTime,
	}
}

// clusterAdminGate rejects the request unless the cluster registry is configured
// and the caller is a platform admin. Cluster-registry management is a
// service-provider concern, so it is gated to rbac.ResourceSystem/ActionAdmin
// (like the kill-switch), not a tenant-level scope.
func (h *handlers) clusterAdminGate(w http.ResponseWriter, r *http.Request) bool {
	if h.deps.Clusters == nil {
		writeError(w, http.StatusNotFound, "cluster registry not configured")
		return false
	}
	if h.rbacEnabled() {
		dec := h.deps.Auth.Authorize(accountFrom(r.Context()), rbac.Request{Resource: rbac.ResourceSystem, Action: rbac.ActionAdmin})
		if !dec.Allowed {
			respondError(w, errForbidden)
			return false
		}
	}
	return true
}

// createCluster registers a cluster. An uploaded "kubeconfig" field means BYOC
// (validated, probed, encrypted, materialized); otherwise it is an operator
// registration referencing an existing "secret_ref".
func (h *handlers) createCluster(w http.ResponseWriter, r *http.Request) {
	if !h.clusterAdminGate(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	entry := clusterregistry.Cluster{
		Name:         r.PostForm.Get("name"),
		IngestURL:    r.PostForm.Get("ingest_url"),
		SidecarImage: r.PostForm.Get("sidecar_image"),
		Namespace:    r.PostForm.Get("namespace"),
		CreatedBy:    accountFrom(r.Context()).Subject,
		CreatedTime:  time.Now().UTC(),
	}

	var (
		out clusterregistry.Cluster
		err error
	)
	if kubeconfig := r.PostForm.Get("kubeconfig"); kubeconfig != "" {
		out, err = h.deps.Clusters.RegisterBYOC(r.Context(), entry, []byte(kubeconfig))
	} else {
		entry.SecretRef = r.PostForm.Get("secret_ref")
		out, err = h.deps.Clusters.RegisterOperator(r.Context(), entry)
	}
	if err != nil {
		respondError(w, err)
		return
	}
	h.audit(r.Context(), "cluster.register", out.Name, string(out.Origin))
	writeJSON(w, http.StatusCreated, toClusterResponse(out))
}

// listClusters returns every registered cluster.
func (h *handlers) listClusters(w http.ResponseWriter, r *http.Request) {
	if !h.clusterAdminGate(w, r) {
		return
	}
	clusters, err := h.deps.Clusters.List(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}
	out := make([]clusterResponse, 0, len(clusters))
	for _, c := range clusters {
		out = append(out, toClusterResponse(c))
	}
	writeJSON(w, http.StatusOK, out)
}

// getCluster returns one registered cluster by name.
func (h *handlers) getCluster(w http.ResponseWriter, r *http.Request) {
	if !h.clusterAdminGate(w, r) {
		return
	}
	c, err := h.deps.Clusters.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toClusterResponse(c))
}

// updateCluster replaces a cluster's mutable deploy settings.
func (h *handlers) updateCluster(w http.ResponseWriter, r *http.Request) {
	if !h.clusterAdminGate(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse form")
		return
	}
	c, err := h.deps.Clusters.Update(r.Context(), r.PathValue("name"),
		r.PostForm.Get("ingest_url"), r.PostForm.Get("sidecar_image"), r.PostForm.Get("namespace"))
	if err != nil {
		respondError(w, err)
		return
	}
	h.audit(r.Context(), "cluster.update", c.Name, "")
	writeJSON(w, http.StatusOK, toClusterResponse(c))
}

// deleteCluster removes a cluster, guarded against an active run.
func (h *handlers) deleteCluster(w http.ResponseWriter, r *http.Request) {
	if !h.clusterAdminGate(w, r) {
		return
	}
	name := r.PathValue("name")
	if err := h.deps.Clusters.Delete(r.Context(), name); err != nil {
		respondError(w, err)
		return
	}
	h.audit(r.Context(), "cluster.delete", name, "")
	w.WriteHeader(http.StatusNoContent)
}
