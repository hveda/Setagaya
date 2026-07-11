package ports

import (
	"errors"
	"net/http"

	"github.com/heridotlife/Setagaya/internal/domain/account"
)

// ErrUnauthenticated is returned by AuthProvider.Authenticate when the request
// carries no valid credentials.
var ErrUnauthenticated = errors.New("ports: unauthenticated")

// AuthProvider resolves the authenticated account from an inbound request. The
// no-auth adapter always returns a fixed admin account; OIDC/token adapters
// validate credentials and map claims to roles.
type AuthProvider interface {
	Authenticate(r *http.Request) (account.Account, error)
}
