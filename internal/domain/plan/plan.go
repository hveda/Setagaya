// Package plan holds the Plan aggregate: a reusable test definition (a JMX plan
// plus data files) that belongs to a Project. Pure domain, no I/O.
package plan

import (
	"errors"
	"strings"
	"time"
)

// MaxNameLen mirrors the persisted schema (plan.name VARCHAR(100)).
const MaxNameLen = 100

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired    = errors.New("plan: name is required")
	ErrNameTooLong     = errors.New("plan: name exceeds maximum length")
	ErrProjectRequired = errors.New("plan: a valid project id is required")
)

// Plan is a test definition owned by a Project.
type Plan struct {
	ID          int64
	Name        string
	ProjectID   int64
	TenantID    *int64
	CreatedBy   string
	UpdatedBy   string
	CreatedTime time.Time
}

// New constructs and validates a Plan. Name is trimmed; ID and CreatedTime are
// assigned by the repository.
func New(name string, projectID int64) (Plan, error) {
	p := Plan{Name: strings.TrimSpace(name), ProjectID: projectID}
	if err := p.Validate(); err != nil {
		return Plan{}, err
	}
	return p, nil
}

// Validate checks the Plan's invariants.
func (p Plan) Validate() error {
	switch {
	case p.Name == "":
		return ErrNameRequired
	case len(p.Name) > MaxNameLen:
		return ErrNameTooLong
	case p.ProjectID <= 0:
		return ErrProjectRequired
	}
	return nil
}
