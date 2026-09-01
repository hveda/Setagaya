package scenarioapp_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/ports"
)

// Fragments whose accept/reject verdict must be identical between
// ValidateRequests (no store) and SetRequests (stores on success). The two
// entry points share requestDiagnostics; this test is the proof they cannot
// drift apart.
func TestValidateRequestsMatchesSetRequests(t *testing.T) {
	t.Parallel()
	fragments := map[string]string{
		"valid":       "requests:\n- url: /a\n",
		"type-error":  "requests:\n- url: /a\n  method: [GET]\n",
		"malformed":   "not: [valid: yaml",
		"no-requests": "default-address: http://x\n",
		"no-url":      "requests:\n  - method: GET\n",
	}
	for name, frag := range fragments {
		svc, _, _ := newScenarioService(t)
		ctx := context.Background()
		p, err := svc.Create(ctx, "smoke", 10)
		if err != nil {
			t.Fatalf("%s: Create: %v", name, err)
		}
		diags, vErr := svc.ValidateRequests(ctx, p.ID, []byte(frag))
		sErr := svc.SetRequests(ctx, p.ID, []byte(frag))
		acceptedByValidate := vErr == nil && len(diags) == 0
		acceptedByStore := sErr == nil
		if acceptedByValidate != acceptedByStore {
			t.Fatalf("%s: validate accepted=%v (diags=%d, err=%v) but store accepted=%v (err=%v)",
				name, acceptedByValidate, len(diags), vErr, acceptedByStore, sErr)
		}
		if !acceptedByStore && !errors.Is(sErr, scenarioapp.ErrRequestsInvalid) {
			t.Fatalf("%s: store rejection = %v, want ErrRequestsInvalid", name, sErr)
		}
	}
}

// TestSetRequestsInvalidCarriesLineDiagnostics covers the G4 contract: a
// rejected fragment comes back as *InvalidRequestsError -- still wrapping
// ErrRequestsInvalid for every existing errors.Is caller -- with one
// diagnostic per yaml.v3 TypeError entry, each anchored to its real line.
func TestSetRequestsInvalidCarriesLineDiagnostics(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()
	p, err := svc.Create(ctx, "smoke", 10)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Type error on line 3 (the method mapping) plus... exactly one entry.
	frag := "requests:\n- url: /a\n  method: [GET]\n"
	var inv *scenarioapp.InvalidRequestsError
	err = svc.SetRequests(ctx, p.ID, []byte(frag))
	if !errors.As(err, &inv) {
		t.Fatalf("SetRequests(type error) = %v, want *InvalidRequestsError", err)
	}
	if !errors.Is(err, scenarioapp.ErrRequestsInvalid) {
		t.Fatalf("SetRequests no longer wraps ErrRequestsInvalid: %v", err)
	}
	if len(inv.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %d entries (%v), want 1", len(inv.Diagnostics), inv.Diagnostics)
	}
	d := inv.Diagnostics[0]
	if d.Line != 3 {
		t.Fatalf("diagnostic line = %d, want 3", d.Line)
	}
	if d.Severity != scenarioapp.SeverityError {
		t.Fatalf("severity = %q, want error", d.Severity)
	}
	if d.Message == "" {
		t.Fatal("diagnostic message is empty")
	}

	// A document that is not YAML at all still anchors to line 1.
	err = svc.SetRequests(ctx, p.ID, []byte("not: [valid: yaml"))
	if !errors.As(err, &inv) {
		t.Fatalf("SetRequests(malformed) = %v, want *InvalidRequestsError", err)
	}
	if len(inv.Diagnostics) != 1 || inv.Diagnostics[0].Line != 1 {
		t.Fatalf("malformed diagnostics = %v, want one entry at line 1", inv.Diagnostics)
	}

	// Semantic rejections anchor to the field they name: a request with no
	// url has no line of its own to point at (path instead), and an empty
	// fragment points at the requests key.
	err = svc.SetRequests(ctx, p.ID, []byte("requests:\n  - method: GET\n"))
	if !errors.As(err, &inv) || len(inv.Diagnostics) != 1 || inv.Diagnostics[0].Path == "" {
		t.Fatalf("no-url diagnostics = %v, want one entry with a path", inv.Diagnostics)
	}
	err = svc.SetRequests(ctx, p.ID, []byte("default-address: http://x\n"))
	if !errors.As(err, &inv) || len(inv.Diagnostics) != 1 || inv.Diagnostics[0].Path != "requests" {
		t.Fatalf("no-requests diagnostics = %v, want one entry at path requests", inv.Diagnostics)
	}
}

// TestValidateRequestsScenarioChecks pins the scenario-level checks both
// entry points share, and that a rejected validate stores nothing.
func TestValidateRequestsScenarioChecks(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()

	// Unknown scenario: ports.ErrNotFound, no diagnostics to render.
	if _, err := svc.ValidateRequests(ctx, 4242, []byte("requests:\n- url: /a\n")); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("ValidateRequests(unknown) = %v, want ErrNotFound", err)
	}

	// Native scenario: the same conflict SetRequests raises.
	p, err := svc.Create(ctx, "native", 10)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.UploadFile(ctx, p.ID, "plan.jmx", strings.NewReader("<jmx/>")); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if _, err := svc.ValidateRequests(ctx, p.ID, []byte("requests:\n- url: /a\n")); !errors.Is(err, scenarioapp.ErrScenarioNotPortable) {
		t.Fatalf("ValidateRequests(native) = %v, want ErrScenarioNotPortable", err)
	}

	// A fragment ValidateRequests rejects leaves the stored fragment
	// untouched (still nothing uploaded here, so Requests stays ErrNotFound).
	p2, err := svc.Create(ctx, "portable", 10)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	diags, vErr := svc.ValidateRequests(ctx, p2.ID, []byte("requests: []"))
	if vErr != nil || len(diags) == 0 {
		t.Fatalf("ValidateRequests(empty requests) = diags %d, err %v, want diagnostics", len(diags), vErr)
	}
	if _, err := svc.Requests(ctx, p2.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Requests after rejected validate = %v, want ErrNotFound (nothing stored)", err)
	}
}
