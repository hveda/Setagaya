package tenant_test

import (
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/tenant"
)

func TestNew_Valid(t *testing.T) {
	t.Parallel()
	tn, err := tenant.New("  ACME-Corp ", "Acme Corporation")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tn.Name != "acme-corp" {
		t.Fatalf("name = %q, want normalized 'acme-corp'", tn.Name)
	}
	if !tn.Active() {
		t.Fatal("new tenant should be active")
	}
}

func TestNew_Invalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, display string
		wantErr       error
	}{
		{"", "Display", tenant.ErrNameRequired},
		{"ab", "Display", tenant.ErrNameInvalid},        // too short
		{"Bad_Name!", "Display", tenant.ErrNameInvalid}, // invalid chars after lowercasing
		{"acme", "", tenant.ErrDisplayNameRequired},
	}
	for _, c := range cases {
		if _, err := tenant.New(c.name, c.display); !errors.Is(err, c.wantErr) {
			t.Errorf("New(%q,%q) err = %v, want %v", c.name, c.display, err, c.wantErr)
		}
	}
}

func TestValidate_Status(t *testing.T) {
	t.Parallel()
	tn := tenant.Tenant{Name: "acme", DisplayName: "Acme", Status: "BOGUS"}
	if err := tn.Validate(); !errors.Is(err, tenant.ErrStatusInvalid) {
		t.Fatalf("err = %v, want ErrStatusInvalid", err)
	}
	tn.Status = tenant.StatusSuspended
	if err := tn.Validate(); err != nil {
		t.Fatalf("suspended should be valid: %v", err)
	}
	if tn.Active() {
		t.Fatal("suspended tenant should not be active")
	}
}

func TestValidate_NameTooLong(t *testing.T) {
	t.Parallel()
	long := make([]byte, tenant.MaxNameLen+1)
	for i := range long {
		long[i] = 'a'
	}
	tn := tenant.Tenant{Name: string(long), DisplayName: "d", Status: tenant.StatusActive}
	if err := tn.Validate(); !errors.Is(err, tenant.ErrNameTooLong) {
		t.Fatalf("err = %v, want ErrNameTooLong", err)
	}
}
