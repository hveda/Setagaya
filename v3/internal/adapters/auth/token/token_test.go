package token_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/adapters/auth/token"
	"github.com/heridotlife/Setagaya/v3/internal/domain/account"
	"github.com/heridotlife/Setagaya/v3/internal/ports/authtest"
)

func req(t *testing.T, authz string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	if authz != "" {
		r.Header.Set("Authorization", authz)
	}
	return r
}

func TestToken_Contract(t *testing.T) {
	t.Parallel()
	authtest.RunAuthProviderContract(t, func(t *testing.T) authtest.Harness {
		p := token.New()
		p.Register("secret", account.Account{Subject: "alice", Email: "a@x"})
		return authtest.Harness{
			Provider:       p,
			ValidRequest:   req(t, "Bearer secret"),
			WantSubject:    "alice",
			InvalidRequest: req(t, "Bearer wrong"),
		}
	})
}

func TestToken_MissingHeaderRejected(t *testing.T) {
	t.Parallel()
	p := token.New()
	p.Register("secret", account.Account{Subject: "alice"})
	if _, err := p.Authenticate(req(t, "")); err == nil {
		t.Fatal("missing header: want error")
	}
	if _, err := p.Authenticate(req(t, "Basic zzz")); err == nil {
		t.Fatal("non-bearer scheme: want error")
	}
}
