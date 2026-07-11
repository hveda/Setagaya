package authapp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/auth/noauth"
	"github.com/heridotlife/Setagaya/internal/adapters/auth/token"
	"github.com/heridotlife/Setagaya/internal/app/authapp"
	"github.com/heridotlife/Setagaya/internal/domain/account"
	"github.com/heridotlife/Setagaya/internal/domain/rbac"
	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

func get() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/projects", nil) }

func TestService_DisabledAllowsEverything(t *testing.T) {
	t.Parallel()
	svc := authapp.NewService(noauth.New("someone"), nil, false)

	if svc.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	// Authenticated but no roles enriched.
	acct, err := svc.Authenticate(get())
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if acct.Subject != "someone" {
		t.Fatalf("subject = %q", acct.Subject)
	}
	// Even an anonymous account is authorized when disabled.
	dec := svc.Authorize(account.Account{}, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionDelete})
	if !dec.Allowed {
		t.Fatalf("disabled Authorize denied: %+v", dec)
	}
}

func TestService_EnabledEnrichesAndAuthorizes(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	ctx := t.Context()
	tid := int64(42)
	if err := store.AssignRole(ctx, ports.RoleGrant{Subject: "alice", RoleName: rbac.RoleTenantEditor, TenantID: &tid}); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	prov := token.New()
	prov.Register("t", account.Account{Subject: "alice", Email: "a@x"})
	svc := authapp.NewService(prov, store, true)

	r := get()
	r.Header.Set("Authorization", "Bearer t")
	acct, err := svc.Authenticate(r)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if !acct.HasTenantAccess(tid) {
		t.Fatalf("grants not merged: %+v", acct)
	}

	// Editor may update within its tenant...
	if dec := svc.Authorize(acct, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionUpdate, TenantID: &tid}); !dec.Allowed {
		t.Fatalf("editor update denied: %+v", dec)
	}
	// ...but not administer it, nor touch a different tenant.
	if dec := svc.Authorize(acct, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionAdmin, TenantID: &tid}); dec.Allowed {
		t.Fatal("editor admin allowed, want denied")
	}
	other := int64(99)
	if dec := svc.Authorize(acct, rbac.Request{Resource: rbac.ResourceProject, Action: rbac.ActionRead, TenantID: &other}); dec.Allowed {
		t.Fatal("cross-tenant read allowed, want denied")
	}
}

func TestService_MergePreservesProviderGlobalRoles(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := authapp.NewService(noauth.New("root"), store, true)

	acct, err := svc.Authenticate(get())
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	// noauth grants service_provider_admin globally; enrichment must keep it.
	if !acct.HasGlobalRole(rbac.RoleServiceProviderAdmin) {
		t.Fatalf("global role lost after merge: %+v", acct)
	}
	if dec := svc.Authorize(acct, rbac.Request{Resource: rbac.ResourceSystem, Action: rbac.ActionAdmin}); !dec.Allowed {
		t.Fatalf("service-provider admin denied: %+v", dec)
	}
}

func TestService_AuthenticateRejectionPropagates(t *testing.T) {
	t.Parallel()
	svc := authapp.NewService(token.New(), fake.NewStore(), true)
	r := get()
	r.Header.Set("Authorization", "Bearer nope")
	if _, err := svc.Authenticate(r); !errors.Is(err, ports.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
}

// errRoles fails RolesFor to drive the enrichment error path.
type errRoles struct{}

var errBoom = errors.New("boom")

func (errRoles) AssignRole(context.Context, ports.RoleGrant) error        { return nil }
func (errRoles) RevokeRole(context.Context, string, string, *int64) error { return nil }
func (errRoles) RolesFor(context.Context, string) (ports.RoleGrants, error) {
	return ports.RoleGrants{}, errBoom
}

func TestService_GrantLookupErrorPropagates(t *testing.T) {
	t.Parallel()
	svc := authapp.NewService(noauth.New("root"), errRoles{}, true)
	if _, err := svc.Authenticate(get()); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
}
