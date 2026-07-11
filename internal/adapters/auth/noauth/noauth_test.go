package noauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heridotlife/Setagaya/internal/adapters/auth/noauth"
	"github.com/heridotlife/Setagaya/internal/domain/rbac"
	"github.com/heridotlife/Setagaya/internal/ports/authtest"
)

func TestNoAuth_Contract(t *testing.T) {
	t.Parallel()
	authtest.RunAuthProviderContract(t, func(t *testing.T) authtest.Harness {
		return authtest.Harness{
			Provider:     noauth.New("setagaya"),
			ValidRequest: httptest.NewRequest(http.MethodGet, "/api/projects", nil),
			WantSubject:  "setagaya",
			// no-auth never rejects
		}
	})
}

func TestNoAuth_DefaultsToServiceProviderAdmin(t *testing.T) {
	t.Parallel()
	acct, _ := noauth.New("").Authenticate(httptest.NewRequest(http.MethodGet, "/", nil))
	if acct.Subject != "setagaya" || !acct.HasGlobalRole(rbac.RoleServiceProviderAdmin) {
		t.Fatalf("default account = %+v", acct)
	}
}
