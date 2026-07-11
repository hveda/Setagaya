// Package tenant holds the Tenant aggregate for multi-tenancy: an isolation
// boundary that owns projects and scopes role grants. Pure domain, no I/O.
package tenant

import (
	"errors"
	"strings"
	"time"
)

// MaxNameLen mirrors the persisted schema.
const MaxNameLen = 50

// Statuses.
const (
	StatusActive    = "ACTIVE"
	StatusSuspended = "SUSPENDED"
)

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired        = errors.New("tenant: name is required")
	ErrNameTooLong         = errors.New("tenant: name exceeds maximum length")
	ErrNameInvalid         = errors.New("tenant: name must be lowercase letters, digits, and hyphens")
	ErrDisplayNameRequired = errors.New("tenant: display name is required")
	ErrStatusInvalid       = errors.New("tenant: invalid status")
)

// Tenant is an isolation boundary owning projects and scoping role grants.
type Tenant struct {
	ID          int64
	Name        string
	DisplayName string
	Status      string
	CreatedTime time.Time
}

// New constructs and validates a Tenant. Name is trimmed and lowercased; ID and
// CreatedTime are assigned by the repository. New tenants start ACTIVE.
func New(name, displayName string) (Tenant, error) {
	t := Tenant{
		Name:        strings.ToLower(strings.TrimSpace(name)),
		DisplayName: strings.TrimSpace(displayName),
		Status:      StatusActive,
	}
	if err := t.Validate(); err != nil {
		return Tenant{}, err
	}
	return t, nil
}

// Validate checks the Tenant's invariants.
func (t Tenant) Validate() error {
	switch {
	case t.Name == "":
		return ErrNameRequired
	case len(t.Name) > MaxNameLen:
		return ErrNameTooLong
	case !validName(t.Name):
		return ErrNameInvalid
	case t.DisplayName == "":
		return ErrDisplayNameRequired
	case !validStatus(t.Status):
		return ErrStatusInvalid
	}
	return nil
}

// Active reports whether the tenant may be used.
func (t Tenant) Active() bool { return t.Status == StatusActive }

func validName(name string) bool {
	if len(name) < 3 {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

func validStatus(s string) bool {
	return s == StatusActive || s == StatusSuspended
}
