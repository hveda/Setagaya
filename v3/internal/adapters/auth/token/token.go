// Package token is a ports.AuthProvider backed by a static map of bearer tokens
// to accounts. It is used in tests and simple deployments, and demonstrates the
// authenticate-and-reject path the OIDC adapter also follows.
package token

import (
	"net/http"
	"strings"
	"sync"

	"github.com/heridotlife/Setagaya/v3/internal/domain/account"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// Provider maps bearer tokens to accounts.
type Provider struct {
	mu       sync.RWMutex
	accounts map[string]account.Account
}

var _ ports.AuthProvider = (*Provider)(nil)

// New returns an empty Provider.
func New() *Provider {
	return &Provider{accounts: map[string]account.Account{}}
}

// Register associates a bearer token with an account.
func (p *Provider) Register(token string, acct account.Account) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.accounts[token] = acct
}

// Authenticate resolves the account from the request's bearer token, or returns
// ErrUnauthenticated.
func (p *Provider) Authenticate(r *http.Request) (account.Account, error) {
	token := bearer(r)
	if token == "" {
		return account.Account{}, ports.ErrUnauthenticated
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	acct, ok := p.accounts[token]
	if !ok {
		return account.Account{}, ports.ErrUnauthenticated
	}
	return acct, nil
}

// bearer extracts the token from an "Authorization: Bearer <token>" header.
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}
