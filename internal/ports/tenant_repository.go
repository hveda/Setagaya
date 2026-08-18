package ports

import (
	"context"

	"github.com/heridotlife/honryu/internal/domain/tenant"
)

// TenantRepository persists tenants.
type TenantRepository interface {
	// CreateTenant inserts a tenant and returns its id. A duplicate name returns
	// ErrFileExists (reused as the generic uniqueness-conflict sentinel).
	CreateTenant(ctx context.Context, t tenant.Tenant) (int64, error)
	// GetTenant returns the tenant, or ErrNotFound.
	GetTenant(ctx context.Context, id int64) (tenant.Tenant, error)
	// ListTenants returns all tenants ordered by id.
	ListTenants(ctx context.Context) ([]tenant.Tenant, error)
	// SetTenantStatus updates a tenant's lifecycle status.
	SetTenantStatus(ctx context.Context, id int64, status string) error
}
