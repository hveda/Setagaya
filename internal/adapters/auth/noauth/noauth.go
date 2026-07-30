// Package noauth is the default ports.AuthProvider for local development: it
// authenticates every request as a fixed service-provider admin account, so the
// platform is fully usable without configuring an identity provider.
package noauth

import (
	"net/http"

	"github.com/heridotlife/Setagaya/internal/domain/account"
	"github.com/heridotlife/Setagaya/internal/domain/rbac"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Provider authenticates all requests as one account.
type Provider struct {
	acct account.Account
}

var _ ports.AuthProvider = (*Provider)(nil)

// New returns a Provider that authenticates every request as subject with the
// given global roles. With no roles, the account is a service-provider admin.
func New(subject string, globalRoles ...string) *Provider {
	if subject == "" {
		subject = "honryu"
	}
	if len(globalRoles) == 0 {
		globalRoles = []string{rbac.RoleServiceProviderAdmin}
	}
	return &Provider{acct: account.Account{
		Subject: subject,
		Name:    subject,
		Global:  globalRoles,
	}}
}

// Authenticate always returns the configured account.
func (p *Provider) Authenticate(*http.Request) (account.Account, error) {
	return p.acct, nil
}
