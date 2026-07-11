package ports

import (
	"context"
	"time"
)

// AuditEvent is a record of a security-relevant administrative action.
type AuditEvent struct {
	Time   time.Time
	Actor  string // subject who performed the action ("" if anonymous)
	Action string // e.g. "tenant.create", "role.assign", "role.revoke"
	Target string // the object acted upon (tenant name, subject, ...)
	Detail string // optional extra context
}

// AuditLog persists audit events. Implementations must be safe for concurrent
// use. A nil AuditLog is never passed to adapters; callers use a no-op when
// auditing is disabled.
type AuditLog interface {
	Record(ctx context.Context, e AuditEvent) error
}
