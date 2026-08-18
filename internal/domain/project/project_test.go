package project_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/project"
)

func TestNew_Valid(t *testing.T) {
	t.Parallel()

	p, err := project.New("web-api", "team-loadtest", "12345")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if p.Name != "web-api" {
		t.Errorf("Name = %q, want web-api", p.Name)
	}
	if p.Owner != "team-loadtest" {
		t.Errorf("Owner = %q, want team-loadtest", p.Owner)
	}
	if p.SID != "12345" {
		t.Errorf("SID = %q, want 12345", p.SID)
	}
	if p.ID != 0 {
		t.Errorf("ID = %d, want 0 (assigned by repository)", p.ID)
	}
}

func TestNew_TrimsWhitespace(t *testing.T) {
	t.Parallel()

	p, err := project.New("  web-api  ", "  team  ", "")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if p.Name != "web-api" {
		t.Errorf("Name = %q, want trimmed web-api", p.Name)
	}
	if p.Owner != "team" {
		t.Errorf("Owner = %q, want trimmed team", p.Owner)
	}
}

func TestNew_EmptySIDIsAllowed(t *testing.T) {
	t.Parallel()

	if _, err := project.New("web-api", "team", ""); err != nil {
		t.Fatalf("New with empty SID: unexpected error: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		pName   string
		owner   string
		sid     string
		wantErr error
	}{
		{"empty name", "", "team", "", project.ErrNameRequired},
		{"blank name", "   ", "team", "", project.ErrNameRequired},
		{"name too long", strings.Repeat("a", 101), "team", "", project.ErrNameTooLong},
		{"empty owner", "web", "", "", project.ErrOwnerRequired},
		{"owner too long", "web", strings.Repeat("o", 51), "", project.ErrOwnerTooLong},
		{"sid not numeric", "web", "team", "abc", project.ErrSIDInvalid},
		{"sid too long", "web", "team", strings.Repeat("9", 26), project.ErrSIDTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := project.New(tc.pName, tc.owner, tc.sid)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("New(%q,%q,%q) err = %v, want %v", tc.pName, tc.owner, tc.sid, err, tc.wantErr)
			}
		})
	}
}
