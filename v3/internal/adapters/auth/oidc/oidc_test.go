package oidc_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heridotlife/Setagaya/v3/internal/adapters/auth/oidc"
	"github.com/heridotlife/Setagaya/v3/internal/ports/authtest"
)

const (
	testKid    = "test-key-1"
	testIssuer = "https://issuer.example"
	testAud    = "setagaya"
)

// signer mints signed JWTs for tests.
type signer struct {
	key *rsa.PrivateKey
	kid string
}

func newSigner(t *testing.T) *signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return &signer{key: key, kid: testKid}
}

func b64(v any) string {
	data, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(data)
}

func (s *signer) token(t *testing.T, claims map[string]any) string {
	t.Helper()
	return s.tokenKid(t, s.kid, claims)
}

func (s *signer) tokenKid(t *testing.T, kid string, claims map[string]any) string {
	t.Helper()
	header := b64(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	payload := b64(claims)
	signingInput := header + "." + payload
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (s *signer) keySet() *oidc.StaticKeySet {
	return oidc.NewStaticKeySet(map[string]*rsa.PublicKey{s.kid: &s.key.PublicKey})
}

func stdClaims() map[string]any {
	return map[string]any{
		"iss":   testIssuer,
		"aud":   testAud,
		"sub":   "user-123",
		"email": "user@example.com",
		"name":  "Test User",
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
}

func bearerReq(t *testing.T, tok string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	return r
}

func TestOIDC_Contract(t *testing.T) {
	t.Parallel()
	s := newSigner(t)
	authtest.RunAuthProviderContract(t, func(t *testing.T) authtest.Harness {
		p := oidc.New(s.keySet(), testIssuer, oidc.WithAudience(testAud))
		return authtest.Harness{
			Provider:       p,
			ValidRequest:   bearerReq(t, s.token(t, stdClaims())),
			WantSubject:    "user-123",
			InvalidRequest: bearerReq(t, s.token(t, map[string]any{"iss": "https://evil", "sub": "x", "exp": time.Now().Add(time.Hour).Unix()})),
		}
	})
}

func TestOIDC_MapsIdentityClaims(t *testing.T) {
	t.Parallel()
	s := newSigner(t)
	p := oidc.New(s.keySet(), testIssuer, oidc.WithAudience(testAud))
	acct, err := p.Authenticate(bearerReq(t, s.token(t, stdClaims())))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if acct.Subject != "user-123" || acct.Email != "user@example.com" || acct.Name != "Test User" {
		t.Fatalf("account = %+v", acct)
	}
}

func TestOIDC_Rejections(t *testing.T) {
	t.Parallel()
	s := newSigner(t)
	p := oidc.New(s.keySet(), testIssuer, oidc.WithAudience(testAud))

	expired := stdClaims()
	expired["exp"] = time.Now().Add(-time.Hour).Unix()

	wrongAud := stdClaims()
	wrongAud["aud"] = "other-service"

	noSub := stdClaims()
	delete(noSub, "sub")

	tests := map[string]string{
		"missing header":    "",
		"malformed token":   "not-a-jwt",
		"bad base64 header": "!!!.eyJhIjoxfQ.sig",
		"bad base64 sig":    b64(map[string]string{"alg": "RS256", "kid": testKid}) + ".eyJhIjoxfQ.!!!",
		"expired":           s.token(t, expired),
		"wrong audience":    s.token(t, wrongAud),
		"no subject":        s.token(t, noSub),
		"unknown kid":       s.tokenKid(t, "other-kid", stdClaims()),
	}
	for name, tok := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := p.Authenticate(bearerReq(t, tok)); err == nil {
				t.Fatal("want rejection, got nil error")
			}
		})
	}
}

func TestOIDC_TamperedPayloadRejected(t *testing.T) {
	t.Parallel()
	s := newSigner(t)
	p := oidc.New(s.keySet(), testIssuer)
	parts := strings.Split(s.token(t, stdClaims()), ".")
	// Rewrite the payload so it no longer matches the signature.
	parts[1] = mangle(parts[1])
	tampered := parts[0] + "." + parts[1] + "." + parts[2]
	if _, err := p.Authenticate(bearerReq(t, tampered)); err == nil {
		t.Fatal("tampered payload accepted")
	}
}

func TestOIDC_AudienceArray(t *testing.T) {
	t.Parallel()
	s := newSigner(t)
	p := oidc.New(s.keySet(), testIssuer, oidc.WithAudience(testAud))
	cl := stdClaims()
	cl["aud"] = []string{"other", testAud}
	if _, err := p.Authenticate(bearerReq(t, s.token(t, cl))); err != nil {
		t.Fatalf("array audience rejected: %v", err)
	}
}

func TestParseJWKS(t *testing.T) {
	t.Parallel()
	s := newSigner(t)
	pub := s.key.PublicKey
	jwks := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": testKid,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
	data, _ := json.Marshal(jwks)
	ks, err := oidc.ParseJWKS(data)
	if err != nil {
		t.Fatalf("ParseJWKS: %v", err)
	}
	p := oidc.New(ks, testIssuer, oidc.WithAudience(testAud), oidc.WithClock(time.Now))
	if _, err := p.Authenticate(bearerReq(t, s.token(t, stdClaims()))); err != nil {
		t.Fatalf("Authenticate with parsed JWKS: %v", err)
	}

	if _, err := oidc.ParseJWKS([]byte("not json")); err == nil {
		t.Fatal("ParseJWKS(bad) = nil error, want error")
	}
}

// mangle changes the first character of a base64url segment to a different one,
// corrupting the decoded bytes.
func mangle(s string) string {
	b := []byte(s)
	if b[0] == 'a' {
		b[0] = 'b'
	} else {
		b[0] = 'a'
	}
	return string(b)
}
