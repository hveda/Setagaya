// Package account holds the authenticated-principal domain: who a caller is and
// which roles they hold, globally and per tenant. Pure, no I/O.
package account

import "sort"

// Account is an authenticated principal and its role assignments. The zero
// value is the unauthenticated (anonymous) account.
type Account struct {
	// Subject is the stable unique id of the principal (OIDC "sub" or username).
	Subject string
	Email   string
	Name    string
	// Global are role names granted across all tenants and global resources.
	Global []string
	// Tenants maps a tenant id to the role names granted within it.
	Tenants map[int64][]string
}

// IsZero reports whether this is the anonymous account.
func (a Account) IsZero() bool { return a.Subject == "" }

// HasGlobalRole reports whether the account holds the named role globally.
func (a Account) HasGlobalRole(name string) bool {
	for _, r := range a.Global {
		if r == name {
			return true
		}
	}
	return false
}

// RolesInTenant returns the role names the account holds within a tenant.
func (a Account) RolesInTenant(tenantID int64) []string {
	return a.Tenants[tenantID]
}

// HasTenantAccess reports whether the account holds any role within a tenant.
func (a Account) HasTenantAccess(tenantID int64) bool {
	return len(a.Tenants[tenantID]) > 0
}

// TenantIDs returns the sorted tenant ids the account has any role in.
func (a Account) TenantIDs() []int64 {
	ids := make([]int64, 0, len(a.Tenants))
	for id, roles := range a.Tenants {
		if len(roles) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
