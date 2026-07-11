// Package authtest holds the shared behavioural contract every
// ports.AuthProvider must satisfy.
package authtest

import (
	"errors"
	"net/http"
	"testing"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// Harness wires a provider under test with a request that should authenticate
// and, optionally, one that should be rejected (nil when the provider never
// rejects, e.g. no-auth).
type Harness struct {
	Provider       ports.AuthProvider
	ValidRequest   *http.Request
	WantSubject    string
	InvalidRequest *http.Request // nil if the provider authenticates everything
}

// NewHarness builds a fresh Harness for one test.
type NewHarness func(t *testing.T) Harness

// RunAuthProviderContract pins authenticate-and-reject behaviour.
func RunAuthProviderContract(t *testing.T, newHarness NewHarness) {
	t.Helper()

	t.Run("valid request authenticates", func(t *testing.T) {
		h := newHarness(t)
		acct, err := h.Provider.Authenticate(h.ValidRequest)
		if err != nil {
			t.Fatalf("Authenticate(valid): %v", err)
		}
		if acct.IsZero() {
			t.Fatal("authenticated account is zero")
		}
		if h.WantSubject != "" && acct.Subject != h.WantSubject {
			t.Fatalf("subject = %q, want %q", acct.Subject, h.WantSubject)
		}
	})

	t.Run("invalid request is rejected", func(t *testing.T) {
		h := newHarness(t)
		if h.InvalidRequest == nil {
			t.Skip("provider authenticates all requests")
		}
		if _, err := h.Provider.Authenticate(h.InvalidRequest); !errors.Is(err, ports.ErrUnauthenticated) {
			t.Fatalf("Authenticate(invalid) err = %v, want ErrUnauthenticated", err)
		}
	})
}
