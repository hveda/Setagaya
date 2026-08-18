package account_test

import (
	"reflect"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/account"
)

func TestAccount_Zero(t *testing.T) {
	t.Parallel()
	if !(account.Account{}).IsZero() {
		t.Fatal("empty account should be zero")
	}
	if (account.Account{Subject: "u"}).IsZero() {
		t.Fatal("account with subject should not be zero")
	}
}

func TestAccount_Roles(t *testing.T) {
	t.Parallel()
	a := account.Account{
		Subject: "u1",
		Global:  []string{"service_provider_admin"},
		Tenants: map[int64][]string{
			5: {"tenant_editor", "tenant_viewer"},
			9: {"tenant_viewer"},
		},
	}

	if !a.HasGlobalRole("service_provider_admin") {
		t.Error("expected global role")
	}
	if a.HasGlobalRole("tenant_editor") {
		t.Error("tenant role should not be global")
	}
	if got := a.RolesInTenant(5); !reflect.DeepEqual(got, []string{"tenant_editor", "tenant_viewer"}) {
		t.Errorf("RolesInTenant(5) = %v", got)
	}
	if !a.HasTenantAccess(5) || a.HasTenantAccess(7) {
		t.Error("tenant access wrong")
	}
	if got := a.TenantIDs(); !reflect.DeepEqual(got, []int64{5, 9}) {
		t.Errorf("TenantIDs = %v, want [5 9]", got)
	}
}

func TestAccount_TenantIDsIgnoresEmpty(t *testing.T) {
	t.Parallel()
	a := account.Account{Subject: "u", Tenants: map[int64][]string{3: {}, 4: {"tenant_viewer"}}}
	if got := a.TenantIDs(); !reflect.DeepEqual(got, []int64{4}) {
		t.Errorf("TenantIDs = %v, want [4]", got)
	}
}
