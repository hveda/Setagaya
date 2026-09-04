package session_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/adapters/auth/session"
	"github.com/heridotlife/honryu/internal/domain/rbac"
	"github.com/heridotlife/honryu/internal/ports/authtest"
)

func testProfiles() []session.Profile {
	return []session.Profile{
		{
			ID: "alice", Name: "Alice", Subject: "alice", Email: "alice@x",
			Global: []string{rbac.RoleServiceProviderAdmin},
		},
		{
			ID: "dave", Name: "Dave", Subject: "dave",
			Tenants: map[int64][]string{1: {rbac.RoleCampaignManager}, 2: {rbac.RoleCampaignManager}},
		},
	}
}

func newProvider(t *testing.T) *session.Provider {
	t.Helper()
	p, err := session.New([]byte("test-signing-key-32-bytes-aaaaaa"), testProfiles())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func cookieReq(t *testing.T, value string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	if value != "" {
		r.AddCookie(&http.Cookie{Name: session.CookieName, Value: value})
	}
	return r
}

func TestSession_Contract(t *testing.T) {
	t.Parallel()
	authtest.RunAuthProviderContract(t, func(t *testing.T) authtest.Harness {
		p := newProvider(t)
		value, err := p.Issue("dave")
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		return authtest.Harness{
			Provider:       p,
			ValidRequest:   cookieReq(t, value),
			WantSubject:    "dave",
			InvalidRequest: cookieReq(t, "not-a-session"),
		}
	})
}

func TestSession_AuthenticateReconstructsAccount(t *testing.T) {
	t.Parallel()
	p := newProvider(t)
	value, err := p.Issue("dave")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	acct, err := p.Authenticate(cookieReq(t, value))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if acct.Subject != "dave" || acct.Name != "Dave" {
		t.Fatalf("account = %+v, want dave/Dave", acct)
	}
	if len(acct.Tenants) != 2 || len(acct.Tenants[1]) != 1 || acct.Tenants[1][0] != rbac.RoleCampaignManager {
		t.Fatalf("tenants = %v, want campaign_manager in 1 and 2", acct.Tenants)
	}
	if len(acct.Global) != 0 {
		t.Fatalf("global = %v, want none", acct.Global)
	}
}

func TestSession_RejectsTamperedAndForged(t *testing.T) {
	t.Parallel()
	p := newProvider(t)
	value, err := p.Issue("alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	body, sig, _ := strings.Cut(value, ".")

	// A different signing key forges nothing.
	rogue, err := session.New([]byte("rogue-key-entirely-different-bytes"), testProfiles())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := rogue.Authenticate(cookieReq(t, value)); err == nil {
		t.Fatal("cookie signed by another key authenticated")
	}

	for name, tampered := range map[string]string{
		"swapped signature": sig[:len(sig)-2] + "AA",
		"payload rewrite":   "eyJzdWIiOiJkYXZlIn0." + sig,
		"no signature":      body,
		"garbage":           "!!!.???",
	} {
		if _, err := p.Authenticate(cookieReq(t, tampered)); err == nil {
			t.Fatalf("%s: authenticated, want rejection", name)
		}
	}
}

func TestSession_RejectsExpired(t *testing.T) {
	t.Parallel()
	p := newProvider(t)
	now := time.Now()
	p.SetNow(func() time.Time { return now })

	value, err := p.Issue("alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Just inside the TTL: valid.
	p.SetNow(func() time.Time { return now.Add(session.TTL - time.Second) })
	if _, err := p.Authenticate(cookieReq(t, value)); err != nil {
		t.Fatalf("inside TTL: %v", err)
	}
	// Past it: rejected.
	p.SetNow(func() time.Time { return now.Add(session.TTL + time.Second) })
	if _, err := p.Authenticate(cookieReq(t, value)); err == nil {
		t.Fatal("expired cookie authenticated, want rejection")
	}
}

func TestSession_IssueUnknownProfile(t *testing.T) {
	t.Parallel()
	p := newProvider(t)
	if _, err := p.Issue("nobody"); err == nil {
		t.Fatal("Issue(unknown) succeeded, want error")
	}
}

func TestSession_ProfilesListsInOrder(t *testing.T) {
	t.Parallel()
	p := newProvider(t)
	got := p.Profiles()
	if len(got) != 2 || got[0].ID != "alice" || got[1].ID != "dave" {
		t.Fatalf("Profiles() = %+v, want alice then dave", got)
	}
}

func TestSession_NewRejectsBadConfig(t *testing.T) {
	t.Parallel()
	if _, err := session.New(nil, testProfiles()); err == nil {
		t.Fatal("empty signing key accepted")
	}
	if _, err := session.New([]byte("k"), nil); err == nil {
		t.Fatal("no profiles accepted")
	}
	dup := testProfiles()
	dup[1].ID = "alice"
	if _, err := session.New([]byte("k"), dup); err == nil {
		t.Fatal("duplicate profile ids accepted")
	}
}
