// Package oidc is a ports.AuthProvider that authenticates callers by verifying
// an OIDC ID token (a signed JWT) presented as a bearer token. It verifies the
// RS256 signature against a JSON Web Key Set, checks the issuer/audience/expiry
// claims, and maps the standard identity claims (sub/email/name) onto an
// account. Authorization roles are resolved separately from the RBAC store, so
// this adapter is concerned only with proving who the caller is.
package oidc

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/heridotlife/honryu/internal/domain/account"
	"github.com/heridotlife/honryu/internal/ports"
)

// KeySet resolves a signing key by its "kid" header.
type KeySet interface {
	Key(kid string) (*rsa.PublicKey, bool)
}

// StaticKeySet is an in-memory KeySet, typically parsed from a JWKS document.
type StaticKeySet struct {
	keys map[string]*rsa.PublicKey
}

// NewStaticKeySet builds a KeySet from a kid->key map.
func NewStaticKeySet(keys map[string]*rsa.PublicKey) *StaticKeySet {
	return &StaticKeySet{keys: keys}
}

// Key returns the key for kid, if present.
func (k *StaticKeySet) Key(kid string) (*rsa.PublicKey, bool) {
	key, ok := k.keys[kid]
	return key, ok
}

// jwk is a single RSA JSON Web Key.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// ParseJWKS parses a JSON Web Key Set of RSA keys into a StaticKeySet.
func ParseJWKS(data []byte) (*StaticKeySet, error) {
	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}
	return NewStaticKeySet(keys), nil
}

// Provider verifies OIDC ID tokens against a KeySet.
type Provider struct {
	keys     KeySet
	issuer   string
	audience string
	now      func() time.Time
}

var _ ports.AuthProvider = (*Provider)(nil)

// Option customizes a Provider.
type Option func(*Provider)

// WithAudience requires the token's "aud" claim to include audience.
func WithAudience(audience string) Option {
	return func(p *Provider) { p.audience = audience }
}

// WithClock overrides the clock used for expiry checks (for tests).
func WithClock(now func() time.Time) Option {
	return func(p *Provider) { p.now = now }
}

// New builds a Provider that trusts tokens signed by keys and issued by issuer.
func New(keys KeySet, issuer string, opts ...Option) *Provider {
	p := &Provider{keys: keys, issuer: issuer, now: time.Now}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// header is the decoded JWT header.
type header struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

// claims is the decoded JWT payload. aud may be a string or an array of
// strings; audience captures both forms.
type claims struct {
	Iss   string   `json:"iss"`
	Sub   string   `json:"sub"`
	Email string   `json:"email"`
	Name  string   `json:"name"`
	Exp   int64    `json:"exp"`
	Aud   audience `json:"aud"`
}

// Authenticate verifies the bearer ID token and returns the caller's account.
func (p *Provider) Authenticate(r *http.Request) (account.Account, error) {
	raw := bearer(r)
	if raw == "" {
		return account.Account{}, ports.ErrUnauthenticated
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return account.Account{}, ports.ErrUnauthenticated
	}

	var hdr header
	if err := decodeSegment(parts[0], &hdr); err != nil || hdr.Alg != "RS256" {
		return account.Account{}, ports.ErrUnauthenticated
	}
	key, ok := p.keys.Key(hdr.Kid)
	if !ok {
		return account.Account{}, ports.ErrUnauthenticated
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return account.Account{}, ports.ErrUnauthenticated
	}
	signingInput := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return account.Account{}, ports.ErrUnauthenticated
	}

	var cl claims
	if err := decodeSegment(parts[1], &cl); err != nil {
		return account.Account{}, ports.ErrUnauthenticated
	}
	if err := p.validate(cl); err != nil {
		return account.Account{}, err
	}
	return account.Account{Subject: cl.Sub, Email: cl.Email, Name: cl.Name}, nil
}

func (p *Provider) validate(cl claims) error {
	if cl.Sub == "" {
		return ports.ErrUnauthenticated
	}
	if p.issuer != "" && cl.Iss != p.issuer {
		return ports.ErrUnauthenticated
	}
	if p.audience != "" && !cl.Aud.contains(p.audience) {
		return ports.ErrUnauthenticated
	}
	if cl.Exp != 0 && p.now().After(time.Unix(cl.Exp, 0)) {
		return ports.ErrUnauthenticated
	}
	return nil
}

// audience decodes a JWT "aud" claim that may be a string or an array.
type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*a = audience{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func (a audience) contains(v string) bool {
	for _, s := range a {
		if s == v {
			return true
		}
	}
	return false
}

func decodeSegment(seg string, v any) error {
	data, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// bearer extracts the token from an "Authorization: Bearer <token>" header.
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}
