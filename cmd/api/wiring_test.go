package main

import (
	"context"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heridotlife/honryu/internal/config"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func TestNewAuthProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// No-auth mode returns a provider without touching the network.
	if p, err := newAuthProvider(ctx, config.AuthConfig{Mode: "none"}); err != nil || p == nil {
		t.Fatalf("newAuthProvider(none) = %v, %v", p, err)
	}

	// Unknown mode errors.
	if _, err := newAuthProvider(ctx, config.AuthConfig{Mode: "ldap"}); err == nil {
		t.Fatal("newAuthProvider(ldap): expected error, got nil")
	}

	// OIDC mode fetches and parses the JWKS from the configured endpoint.
	jwks := `{"keys":[{"kty":"RSA","kid":"k1","n":"` +
		base64.RawURLEncoding.EncodeToString(big.NewInt(0xC0FFEE).Bytes()) + `","e":"` +
		base64.RawURLEncoding.EncodeToString(big.NewInt(65537).Bytes()) + `"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(jwks))
	}))
	defer srv.Close()
	if p, err := newAuthProvider(ctx, config.AuthConfig{
		Mode: "oidc", OIDC: config.OIDCConfig{Issuer: "https://x", JWKSURL: srv.URL},
	}); err != nil || p == nil {
		t.Fatalf("newAuthProvider(oidc) = %v, %v", p, err)
	}

	// A failing JWKS endpoint surfaces an error.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	if _, err := newAuthProvider(ctx, config.AuthConfig{
		Mode: "oidc", OIDC: config.OIDCConfig{Issuer: "https://x", JWKSURL: bad.URL},
	}); err == nil {
		t.Fatal("newAuthProvider(oidc) with 500 JWKS: expected error, got nil")
	}
}

func TestNewScheduler(t *testing.T) {
	t.Parallel()

	if s, err := newScheduler(config.ClusterConfig{Scheduler: "fake"}, fake.NewStore()); err != nil || s == nil {
		t.Fatalf("newScheduler(fake) = %v, %v", s, err)
	}
	// k8s outside a cluster fails to load in-cluster config: covers that branch.
	if _, err := newScheduler(config.ClusterConfig{Scheduler: "k8s", Namespace: "default", EnginePort: 8080}, fake.NewStore()); err == nil {
		t.Fatal("newScheduler(k8s) outside cluster: expected error, got nil")
	}
	if _, err := newScheduler(config.ClusterConfig{Scheduler: "nope"}, fake.NewStore()); err == nil {
		t.Fatal("newScheduler(nope): expected error, got nil")
	}
}

func TestNewObjectStore(t *testing.T) {
	t.Parallel()

	if s, err := newObjectStore(config.StorageConfig{Driver: "local", Root: t.TempDir()}); err != nil || s == nil {
		t.Fatalf("newObjectStore(local) = %v, %v", s, err)
	}
	s, err := newObjectStore(config.StorageConfig{
		Driver: "nexus", BaseURL: "https://nexus.example", Repo: "raw", Username: "u", Password: "p",
	})
	if err != nil || s == nil {
		t.Fatalf("newObjectStore(nexus) = %v, %v", s, err)
	}
	if got := s.URL("scenario/1/a.jmx"); got != "https://nexus.example/repository/raw/scenario/1/a.jmx" {
		t.Fatalf("nexus URL = %q", got)
	}
	if _, err := newObjectStore(config.StorageConfig{Driver: "s3"}); err == nil {
		t.Fatal("newObjectStore(s3): expected error, got nil")
	}
}

// The cluster registry is wired only for the k8s scheduler with a credential
// key, and that path needs the in-cluster config -- absent in a unit test
// environment, so the error must surface rather than silently disabling the
// endpoints. Anything else returns a nil service (endpoints disabled), not an
// error: a deployment not registering clusters need not configure a key.
func TestNewClusterService(t *testing.T) {
	t.Parallel()

	if _, err := newClusterService(config.ClusterConfig{Scheduler: "k8s", CredentialKey: "0123456789abcdef"}, fake.NewStore()); err == nil {
		t.Fatal("newClusterService(k8s) outside a cluster: expected error, got nil")
	}
	for _, cfg := range []config.ClusterConfig{
		{Scheduler: "fake", CredentialKey: "0123456789abcdef"},
		{Scheduler: "k8s"}, // no credential key: not registering clusters
	} {
		if svc, err := newClusterService(cfg, fake.NewStore()); err != nil || svc != nil {
			t.Fatalf("newClusterService(%+v) = %v, %v, want nil, nil", cfg, svc, err)
		}
	}
}

func TestFetchJWKS_Errors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// A control character makes request construction itself fail.
	if _, err := fetchJWKS(ctx, "http://127.0.0.1/\x7f"); err == nil {
		t.Fatal("fetchJWKS with an invalid URL: expected error, got nil")
	}
	// Nothing listens on port 1: the request fails in transport.
	if _, err := fetchJWKS(ctx, "http://127.0.0.1:1/jwks"); err == nil {
		t.Fatal("fetchJWKS with an unreachable endpoint: expected error, got nil")
	}
}
