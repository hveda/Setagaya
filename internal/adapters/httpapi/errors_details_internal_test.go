package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/app/quotaapp"
)

// respondError's details envelope, driven directly: tenant_quota_test.go
// covers the full wiring; these pin the hint variants (set vs
// unconfigured ceiling) and the bare-sentinel fallback without building a
// router.

// quotaDetailsBody mirrors the 429 envelope respondError emits.
type quotaDetailsBody struct {
	Message string `json:"message"`
	Details struct {
		TenantID  int64  `json:"tenant_id"`
		Cluster   string `json:"cluster"`
		Requested int    `json:"requested"`
		Used      int    `json:"used"`
		Ceiling   int    `json:"ceiling"`
		Hint      string `json:"hint"`
	} `json:"details"`
}

func TestRespondError_OverQuotaDetails(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		err      error
		wantUsed int
		wantCeil int
		wantHint string
	}{
		{
			name:     "configured ceiling",
			err:      &quotaapp.OverQuotaError{TenantID: 7, Cluster: "prod", Requested: 4, Used: 6, Ceiling: 9},
			wantUsed: 6,
			wantCeil: 9,
			wantHint: "PUT /api/tenants/{tenant_id}/quota ceiling=9",
		},
		{
			name:     "unconfigured ceiling",
			err:      &quotaapp.OverQuotaError{TenantID: 7, Cluster: "prod", Requested: 4, Used: 0, Ceiling: 0, NoQuotaConfigured: true},
			wantUsed: 0,
			wantCeil: 0,
			wantHint: "PUT /api/tenants/{tenant_id}/quota ceiling=0; no quota row exists for this tenant+cluster",
		},
		{
			// Callers re-wrap (Trigger's own %w chains); the typed form must
			// survive the wrap for the details to survive it too.
			name:     "wrapped",
			err:      fmt.Errorf("trigger: %w", &quotaapp.OverQuotaError{TenantID: 7, Cluster: "prod", Requested: 4, Used: 6, Ceiling: 9}),
			wantUsed: 6,
			wantCeil: 9,
			wantHint: "PUT /api/tenants/{tenant_id}/quota ceiling=9",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			respondError(rec, tc.err)
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429", rec.Code)
			}
			var body quotaDetailsBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("429 body is not the envelope: %v (%s)", err, rec.Body.String())
			}
			if body.Message != "reservation would exceed tenant quota" {
				t.Fatalf("message = %q, want the fixed contract", body.Message)
			}
			d := body.Details
			if d.TenantID != 7 || d.Cluster != "prod" || d.Requested != 4 || d.Used != tc.wantUsed || d.Ceiling != tc.wantCeil {
				t.Fatalf("details = %+v, want the typed error's numbers", d)
			}
			if d.Hint != tc.wantHint {
				t.Fatalf("hint = %q, want %q", d.Hint, tc.wantHint)
			}
		})
	}
}

// A bare sentinel (no typed form attached) keeps the message-only envelope:
// "details" is opt-in per error, never an empty object.
func TestRespondError_OverQuotaSentinelWithoutNumbersIsMessageOnly(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	respondError(rec, quotaapp.ErrOverQuota)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"message":"reservation would exceed tenant quota"}` {
		t.Fatalf("bare-sentinel 429 body = %s, want message-only", got)
	}
}

// writeError delegates to writeErrorDetails with nil details, and nil must
// omit the key: message-only bodies stay byte-identical to the
// pre-envelope shape existing consumers pin.
func TestWriteErrorDetails_NilOmitsTheKey(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	writeErrorDetails(rec, http.StatusNotFound, "ports: not found", nil)
	if got := strings.TrimSpace(rec.Body.String()); got != `{"message":"ports: not found"}` {
		t.Fatalf("nil-details body = %s, want message-only byte-identical", got)
	}
}
