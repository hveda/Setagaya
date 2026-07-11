package plan_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/heridotlife/Setagaya/internal/domain/plan"
)

func TestNew_Valid(t *testing.T) {
	t.Parallel()

	p, err := plan.New("  smoke-test  ", 7)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if p.Name != "smoke-test" {
		t.Errorf("Name = %q, want trimmed smoke-test", p.Name)
	}
	if p.ProjectID != 7 {
		t.Errorf("ProjectID = %d, want 7", p.ProjectID)
	}
	if p.ID != 0 {
		t.Errorf("ID = %d, want 0 (assigned by repository)", p.ID)
	}
}

func TestNew_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		planName  string
		projectID int64
		wantErr   error
	}{
		{"empty name", "", 1, plan.ErrNameRequired},
		{"blank name", "   ", 1, plan.ErrNameRequired},
		{"name too long", strings.Repeat("a", 101), 1, plan.ErrNameTooLong},
		{"zero project", "smoke", 0, plan.ErrProjectRequired},
		{"negative project", "smoke", -1, plan.ErrProjectRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := plan.New(tc.planName, tc.projectID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("New(%q,%d) err = %v, want %v", tc.planName, tc.projectID, err, tc.wantErr)
			}
		})
	}
}
