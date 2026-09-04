// Package session is the demo-mode ports.AuthProvider: personas are
// deployment fixtures (Helm values, not domain data), and selecting one mints
// an HMAC-signed cookie that carries the persona's account. Selecting a
// persona IS the authentication, by explicit decision -- which is why the
// whole surface ships behind demo.enabled / AUTH_MODE=demo.
//
// The session is a cookie, never a bearer token: the SPA's Live Status uses
// EventSource, which cannot set an Authorization header, so a bearer token
// would break the stream the moment RBAC gates it.
//
// Cookie format: base64url(JSON payload) + "." + base64url(HMAC-SHA256 over
// the encoded payload). The payload carries the account (subject, name,
// email, global and per-tenant roles) and an expiry; the signature makes the
// payload tamper-proof without a session store, so two API replicas need only
// the shared signing key, and a restart invalidates nothing.
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/heridotlife/honryu/internal/domain/account"
	"github.com/heridotlife/honryu/internal/ports"
)

// CookieName is the demo session cookie's name.
const CookieName = "honryu_session"

// TTL is how long a session lasts; the Set-Cookie's Max-Age matches, so the
// browser drops it at the same moment the server stops honoring it.
const TTL = 8 * time.Hour

// Profile is one demo persona: a fixed, deployment-configured account. ID is
// what POST /api/session names; Subject/Global/Tenants are what the minted
// cookie carries.
type Profile struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Subject string             `json:"subject,omitempty"`
	Email   string             `json:"email,omitempty"`
	Global  []string           `json:"global,omitempty"`
	Tenants map[int64][]string `json:"tenants,omitempty"`
}

// Account renders the persona as an authenticated principal.
func (p Profile) Account() account.Account {
	return account.Account{
		Subject: p.Subject,
		Email:   p.Email,
		Name:    p.Name,
		Global:  p.Global,
		Tenants: p.Tenants,
	}
}

// payload is the signed cookie body: the account plus its expiry.
type payload struct {
	Subject string             `json:"sub"`
	Name    string             `json:"name,omitempty"`
	Email   string             `json:"email,omitempty"`
	Global  []string           `json:"global,omitempty"`
	Tenants map[int64][]string `json:"tenants,omitempty"`
	Expires time.Time          `json:"exp"`
}

// Provider verifies session cookies and mints them for the configured
// personas. It implements ports.AuthProvider.
type Provider struct {
	secret   []byte
	profiles map[string]Profile
	order    []string
	now      func() time.Time
}

var _ ports.AuthProvider = (*Provider)(nil)

// ErrUnknownProfile is returned by Issue for a profile id the deployment does
// not configure.
var ErrUnknownProfile = errors.New("session: unknown profile")

// New builds a Provider over the signing key and persona list. The key is the
// operator-created Secret shared by every API replica; profile ids must be
// unique and non-empty.
func New(secret []byte, profiles []Profile) (*Provider, error) {
	if len(secret) == 0 {
		return nil, errors.New("session: signing key must not be empty")
	}
	if len(profiles) == 0 {
		return nil, errors.New("session: at least one profile is required")
	}
	p := &Provider{
		secret:   secret,
		profiles: make(map[string]Profile, len(profiles)),
		order:    make([]string, 0, len(profiles)),
		now:      time.Now,
	}
	for _, prof := range profiles {
		if prof.ID == "" {
			return nil, errors.New("session: profile with empty id")
		}
		if _, dup := p.profiles[prof.ID]; dup {
			return nil, fmt.Errorf("session: duplicate profile id %q", prof.ID)
		}
		if prof.Subject == "" {
			prof.Subject = prof.ID
		}
		p.profiles[prof.ID] = prof
		p.order = append(p.order, prof.ID)
	}
	return p, nil
}

// SetNow overrides the clock, for tests only.
func (p *Provider) SetNow(now func() time.Time) { p.now = now }

// Issue mints a signed cookie value for the named persona, expiring TTL from
// now.
func (p *Provider) Issue(profileID string) (string, error) {
	prof, ok := p.profiles[profileID]
	if !ok {
		return "", fmt.Errorf("%w %q", ErrUnknownProfile, profileID)
	}
	acct := prof.Account()
	body, err := json.Marshal(payload{
		Subject: acct.Subject,
		Name:    acct.Name,
		Email:   acct.Email,
		Global:  acct.Global,
		Tenants: acct.Tenants,
		Expires: p.now().Add(TTL).UTC(),
	})
	if err != nil {
		return "", fmt.Errorf("session: encode payload: %w", err)
	}
	return p.sign(body), nil
}

// sign encodes body and appends its HMAC-SHA256 signature, both base64url.
func (p *Provider) sign(body []byte) string {
	enc := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, p.secret)
	mac.Write([]byte(enc))
	return enc + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Profiles returns the configured personas in configuration order -- the
// picker's list.
func (p *Provider) Profiles() []Profile {
	out := make([]Profile, 0, len(p.order))
	for _, id := range p.order {
		out = append(out, p.profiles[id])
	}
	return out
}

// Authenticate reconstructs the account from the request's session cookie,
// rejecting anything missing, tampered with, or expired.
func (p *Provider) Authenticate(r *http.Request) (account.Account, error) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return account.Account{}, ports.ErrUnauthenticated
	}
	enc, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return account.Account{}, ports.ErrUnauthenticated
	}
	mac := hmac.New(sha256.New, p.secret)
	mac.Write([]byte(enc))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return account.Account{}, ports.ErrUnauthenticated
	}
	body, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return account.Account{}, ports.ErrUnauthenticated
	}
	var pl payload
	if err := json.Unmarshal(body, &pl); err != nil {
		return account.Account{}, ports.ErrUnauthenticated
	}
	if pl.Subject == "" || !p.now().Before(pl.Expires) {
		return account.Account{}, ports.ErrUnauthenticated
	}
	return account.Account{
		Subject: pl.Subject,
		Email:   pl.Email,
		Name:    pl.Name,
		Global:  pl.Global,
		Tenants: pl.Tenants,
	}, nil
}
