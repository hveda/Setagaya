package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/clusterapp"
	"github.com/heridotlife/honryu/internal/domain/account"
	"github.com/heridotlife/honryu/internal/domain/clusterregistry"
	"github.com/heridotlife/honryu/internal/ports"
)

// stubClusterService is a configurable httpapi.ClusterService for driving each
// handler path and error mapping without the full clusterapp wiring.
type stubClusterService struct {
	registerOperator func(clusterregistry.Cluster) (clusterregistry.Cluster, error)
	registerBYOC     func(clusterregistry.Cluster, []byte) (clusterregistry.Cluster, error)
	get              func(string) (clusterregistry.Cluster, error)
	list             func() ([]clusterregistry.Cluster, error)
	update           func(name, ingest, sidecar, ns string) (clusterregistry.Cluster, error)
	del              func(string) error
}

func (s *stubClusterService) RegisterOperator(_ context.Context, e clusterregistry.Cluster) (clusterregistry.Cluster, error) {
	return s.registerOperator(e)
}
func (s *stubClusterService) RegisterBYOC(_ context.Context, e clusterregistry.Cluster, kc []byte) (clusterregistry.Cluster, error) {
	return s.registerBYOC(e, kc)
}
func (s *stubClusterService) Get(_ context.Context, name string) (clusterregistry.Cluster, error) {
	return s.get(name)
}
func (s *stubClusterService) List(_ context.Context) ([]clusterregistry.Cluster, error) {
	return s.list()
}
func (s *stubClusterService) Update(_ context.Context, name, ingest, sidecar, ns string) (clusterregistry.Cluster, error) {
	return s.update(name, ingest, sidecar, ns)
}
func (s *stubClusterService) Delete(_ context.Context, name string) error { return s.del(name) }

func newClusterRouter(svc httpapi.ClusterService) http.Handler {
	return httpapi.NewRouter(httpapi.Deps{
		Clusters:      svc,
		DefaultOwners: []string{"honryu"}, // no-auth: the admin gate passes on rbac-disabled
	})
}

func doForm(t *testing.T, h http.Handler, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateCluster_BYOC_HappyPath(t *testing.T) {
	t.Parallel()
	var gotKubeconfig []byte
	svc := &stubClusterService{
		registerBYOC: func(e clusterregistry.Cluster, kc []byte) (clusterregistry.Cluster, error) {
			gotKubeconfig = kc
			e.Origin = clusterregistry.OriginBYOC
			e.APIURL = "https://byoc:6443"
			e.SecretRef = "honryu-cluster-" + e.Name
			return e, nil
		},
	}
	h := newClusterRouter(svc)

	rec := doForm(t, h, http.MethodPost, "/api/clusters", url.Values{
		"name": {"prod-eu"}, "ingest_url": {"http://ingest"}, "sidecar_image": {"img"},
		"namespace": {"honryu"}, "kubeconfig": {"apiVersion: v1\nkind: Config"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create BYOC = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if string(gotKubeconfig) != "apiVersion: v1\nkind: Config" {
		t.Fatalf("kubeconfig not forwarded: %q", gotKubeconfig)
	}
	if !strings.Contains(rec.Body.String(), `"origin":"byoc"`) {
		t.Fatalf("response missing byoc origin: %s", rec.Body.String())
	}
}

func TestCreateCluster_Operator_UsesSecretRef(t *testing.T) {
	t.Parallel()
	var gotSecretRef string
	svc := &stubClusterService{
		registerOperator: func(e clusterregistry.Cluster) (clusterregistry.Cluster, error) {
			gotSecretRef = e.SecretRef
			e.Origin = clusterregistry.OriginOperator
			return e, nil
		},
	}
	h := newClusterRouter(svc)

	rec := doForm(t, h, http.MethodPost, "/api/clusters", url.Values{
		"name": {"prod-eu"}, "ingest_url": {"http://ingest"}, "sidecar_image": {"img"},
		"namespace": {"honryu"}, "secret_ref": {"existing-secret"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create operator = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if gotSecretRef != "existing-secret" {
		t.Fatalf("secret_ref not forwarded: %q", gotSecretRef)
	}
}

func TestCreateCluster_ErrorMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"validation", clusterregistry.ErrNameRequired, http.StatusBadRequest},
		{"invalid kubeconfig", clusterapp.ErrKubeconfigInvalid, http.StatusBadRequest},
		{"duplicate", ports.ErrClusterExists, http.StatusConflict},
		{"probe failure", &ports.ProbeError{Kind: ports.ProbeUnderPrivileged, Message: "missing configmaps"}, http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := &stubClusterService{
				registerBYOC: func(clusterregistry.Cluster, []byte) (clusterregistry.Cluster, error) {
					return clusterregistry.Cluster{}, tc.err
				},
			}
			h := newClusterRouter(svc)
			rec := doForm(t, h, http.MethodPost, "/api/clusters", url.Values{
				"name": {"prod-eu"}, "ingest_url": {"http://ingest"}, "sidecar_image": {"img"},
				"namespace": {"honryu"}, "kubeconfig": {"kc"},
			})
			if rec.Code != tc.want {
				t.Fatalf("create (%s) = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestGetCluster_NotFound(t *testing.T) {
	t.Parallel()
	svc := &stubClusterService{get: func(string) (clusterregistry.Cluster, error) {
		return clusterregistry.Cluster{}, ports.ErrNotFound
	}}
	h := newClusterRouter(svc)
	rec := doForm(t, h, http.MethodGet, "/api/clusters/ghost", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get(ghost) = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListClusters(t *testing.T) {
	t.Parallel()
	svc := &stubClusterService{list: func() ([]clusterregistry.Cluster, error) {
		return []clusterregistry.Cluster{{Name: "prod-eu", Origin: clusterregistry.OriginOperator}}, nil
	}}
	h := newClusterRouter(svc)
	rec := doForm(t, h, http.MethodGet, "/api/clusters", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"prod-eu"`) {
		t.Fatalf("list body missing cluster: %s", rec.Body.String())
	}
}

func TestDeleteCluster_GuardConflict(t *testing.T) {
	t.Parallel()
	svc := &stubClusterService{del: func(string) error { return clusterapp.ErrClusterInUse }}
	h := newClusterRouter(svc)
	rec := doForm(t, h, http.MethodDelete, "/api/clusters/prod-eu", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete (in use) = %d, want 409 (%s)", rec.Code, rec.Body.String())
	}
}

func TestDeleteCluster_HappyPath(t *testing.T) {
	t.Parallel()
	svc := &stubClusterService{del: func(string) error { return nil }}
	h := newClusterRouter(svc)
	rec := doForm(t, h, http.MethodDelete, "/api/clusters/prod-eu", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204 (%s)", rec.Code, rec.Body.String())
	}
}

func TestClusters_NotConfiguredReturns404(t *testing.T) {
	t.Parallel()
	// No Clusters dep wired.
	h := httpapi.NewRouter(httpapi.Deps{DefaultOwners: []string{"honryu"}})
	rec := doForm(t, h, http.MethodGet, "/api/clusters", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("list (not configured) = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

// Cluster-registry management is platform-admin gated: a tenant admin, however
// privileged within its tenant, is forbidden; a service-provider admin is not.
func TestClusters_RBAC_PlatformAdminGate(t *testing.T) {
	t.Parallel()
	f := newRBACFixture(t)
	acme := createTenant(t, f, "acme", "Acme")
	f.prov.Register("val-tok", account.Account{Subject: "val"})
	assignRole(t, f, acme, "val", "tenant_admin")

	forbidden := f.req(t, http.MethodGet, "/api/clusters", "val-tok", nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("list (tenant admin) = %d, want 403 (%s)", forbidden.Code, forbidden.Body.String())
	}
	allowed := f.req(t, http.MethodGet, "/api/clusters", "admin-tok", nil)
	if allowed.Code != http.StatusOK {
		t.Fatalf("list (SP admin) = %d, want 200 (%s)", allowed.Code, allowed.Body.String())
	}
}
