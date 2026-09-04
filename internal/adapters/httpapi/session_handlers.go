package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/heridotlife/honryu/internal/adapters/auth/session"
	"github.com/heridotlife/honryu/internal/domain/account"
)

// maxSessionBody bounds the POST /api/session body: one profile id.
const maxSessionBody = 1 << 10

// createSession authenticates as a demo persona: it mints the HMAC-signed
// honryu_session cookie. The route is public -- selecting a persona IS the
// authentication, which is exactly why demo mode ships off by default.
func (h *handlers) createSession(w http.ResponseWriter, r *http.Request) {
	if h.deps.Sessions == nil {
		writeError(w, http.StatusNotFound, "demo sessions not configured")
		return
	}
	var body struct {
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSessionBody)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "body must be JSON {\"profile\": id}")
		return
	}
	value, err := h.deps.Sessions.Issue(body.Profile)
	if err != nil {
		if errors.Is(err, session.ErrUnknownProfile) {
			writeError(w, http.StatusNotFound, "unknown profile")
			return
		}
		writeError(w, http.StatusInternalServerError, "issue session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     session.CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(session.TTL.Seconds()),
		HttpOnly: true, // the SPA never reads the cookie; scripts must not be able to
		Secure:   true, // the cookie is only ever sent over HTTPS
		// Strict: the session never leaves this origin, so there is no
		// reason a cross-site request should carry it.
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// deleteSession expires the session cookie (logout).
func (h *handlers) deleteSession(w http.ResponseWriter, _ *http.Request) {
	if h.deps.Sessions == nil {
		writeError(w, http.StatusNotFound, "demo sessions not configured")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     session.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1, // delete now
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

// sessionProfile is one picker entry: what the persona is called, and the id
// that selects it. Role detail is not leaked here -- the picker has no use
// for it, and /api/me is the shaped answer once authenticated.
type sessionProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// listSessionProfiles returns the picker's persona list, 404 when demo mode
// is off (the picker is simply not there).
func (h *handlers) listSessionProfiles(w http.ResponseWriter, _ *http.Request) {
	if h.deps.Sessions == nil {
		writeError(w, http.StatusNotFound, "demo sessions not configured")
		return
	}
	configured := h.deps.Sessions.Profiles()
	out := make([]sessionProfile, 0, len(configured))
	for _, p := range configured {
		out = append(out, sessionProfile{ID: p.ID, Name: p.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

// meResponse is GET /api/me: who the caller is and what they may do. The
// permission map is the single source the SPA shapes its UI from.
type meResponse struct {
	Subject     string              `json:"subject"`
	Name        string              `json:"name"`
	Email       string              `json:"email"`
	GlobalRoles []string            `json:"global_roles"`
	Tenants     map[int64][]string  `json:"tenants"`
	Permissions map[string][]string `json:"permissions"`
	Demo        bool                `json:"demo"`
}

// me answers with the authenticated account and its permission map. It needs
// no new permission concept: any authenticated caller may see itself.
func (h *handlers) me(w http.ResponseWriter, r *http.Request) {
	if h.deps.Auth == nil {
		writeError(w, http.StatusNotFound, "auth not configured")
		return
	}
	var acct account.Account
	if h.rbacEnabled() {
		// The middleware already authenticated and enriched the account.
		acct = accountFrom(r.Context())
	} else {
		// Legacy mode: ask the provider directly so the SPA has one stable
		// endpoint in every mode.
		var err error
		if acct, err = h.deps.Auth.Authenticate(r); err != nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
	}
	writeJSON(w, http.StatusOK, meResponse{
		Subject:     acct.Subject,
		Name:        acct.Name,
		Email:       acct.Email,
		GlobalRoles: acct.Global,
		Tenants:     acct.Tenants,
		Permissions: h.deps.Auth.Permissions(acct),
		Demo:        h.deps.Sessions != nil,
	})
}
