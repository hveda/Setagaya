// Package authapp is the authentication/authorization use-case. It wraps a
// pluggable ports.AuthProvider, enriches the authenticated account with
// persisted role grants, and answers authorization questions against the RBAC
// catalog. When RBAC is disabled it still authenticates, but authorizes
// everything — so the HTTP layer's legacy owner-based checks stay in force.
package authapp

import (
	"net/http"
	"slices"

	"github.com/heridotlife/Setagaya/internal/domain/account"
	"github.com/heridotlife/Setagaya/internal/domain/rbac"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Service is the auth use-case.
type Service struct {
	provider ports.AuthProvider
	roles    ports.RoleAssignmentRepository // may be nil (no persisted grants)
	catalog  map[string]rbac.Role
	enabled  bool
}

// NewService wires the auth service. When enabled is false, Authorize allows
// everything and Authenticate skips grant enrichment. The role catalog defaults
// to rbac.DefaultCatalog.
func NewService(provider ports.AuthProvider, roles ports.RoleAssignmentRepository, enabled bool) *Service {
	return &Service{
		provider: provider,
		roles:    roles,
		catalog:  rbac.DefaultCatalog(),
		enabled:  enabled,
	}
}

// Enabled reports whether RBAC authorization is in force.
func (s *Service) Enabled() bool { return s.enabled }

// Authenticate resolves the account from the request and, when RBAC is enabled,
// merges in the subject's persisted role grants.
func (s *Service) Authenticate(r *http.Request) (account.Account, error) {
	acct, err := s.provider.Authenticate(r)
	if err != nil {
		return account.Account{}, err
	}
	if !s.enabled || s.roles == nil {
		return acct, nil
	}
	grants, err := s.roles.RolesFor(r.Context(), acct.Subject)
	if err != nil {
		return account.Account{}, err
	}
	return mergeGrants(acct, grants), nil
}

// Authorize decides whether acct may perform req. With RBAC disabled every
// request is allowed (the HTTP layer falls back to owner checks).
func (s *Service) Authorize(acct account.Account, req rbac.Request) rbac.Decision {
	if !s.enabled {
		return rbac.Decision{Allowed: true, Reason: "rbac disabled"}
	}
	return rbac.Authorize(acct, s.catalog, req)
}

// mergeGrants folds persisted grants into an account, deduplicating role names.
func mergeGrants(acct account.Account, g ports.RoleGrants) account.Account {
	acct.Global = dedupAppend(acct.Global, g.Global...)
	for tid, roles := range g.Tenants {
		if acct.Tenants == nil {
			acct.Tenants = make(map[int64][]string)
		}
		acct.Tenants[tid] = dedupAppend(acct.Tenants[tid], roles...)
	}
	return acct
}

func dedupAppend(dst []string, vals ...string) []string {
	for _, v := range vals {
		if !slices.Contains(dst, v) {
			dst = append(dst, v)
		}
	}
	return dst
}
