package collection_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/domain/collection"
)

func TestNew_Valid(t *testing.T) {
	t.Parallel()

	c, err := collection.New("  peak-hour  ", 3)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if c.Name != "peak-hour" {
		t.Errorf("Name = %q, want trimmed peak-hour", c.Name)
	}
	if c.ProjectID != 3 {
		t.Errorf("ProjectID = %d, want 3", c.ProjectID)
	}
	if c.CSVSplit {
		t.Errorf("CSVSplit = true, want false by default")
	}
}

func TestNew_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		collName string
		project  int64
		wantErr  error
	}{
		{"empty name", "", 1, collection.ErrNameRequired},
		{"name too long", strings.Repeat("c", 101), 1, collection.ErrNameTooLong},
		{"zero project", "peak", 0, collection.ErrProjectRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := collection.New(tc.collName, tc.project)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("New(%q,%d) err = %v, want %v", tc.collName, tc.project, err, tc.wantErr)
			}
		})
	}
}
