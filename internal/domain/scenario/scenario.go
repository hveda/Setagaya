// Package scenario holds the Scenario aggregate: a reusable workload definition
// (a script or JMX file plus data files) that belongs to a Project. It is the
// Taurus "scenarios" concept. Pure domain, no I/O.
package scenario

import (
	"errors"
	"strings"
	"time"
)

// MaxNameLen mirrors the persisted schema (scenario.name VARCHAR(100)).
const MaxNameLen = 100

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired    = errors.New("scenario: name is required")
	ErrNameTooLong     = errors.New("scenario: name exceeds maximum length")
	ErrProjectRequired = errors.New("scenario: a valid project id is required")
)

// Scenario is a test definition owned by a Project.
type Scenario struct {
	ID          int64
	Name        string
	ProjectID   int64
	TenantID    *int64
	CreatedBy   string
	UpdatedBy   string
	CreatedTime time.Time
}

// New constructs and validates a Scenario. Name is trimmed; ID and CreatedTime are
// assigned by the repository.
func New(name string, projectID int64) (Scenario, error) {
	p := Scenario{Name: strings.TrimSpace(name), ProjectID: projectID}
	if err := p.Validate(); err != nil {
		return Scenario{}, err
	}
	return p, nil
}

// Validate checks the Scenario's invariants.
func (p Scenario) Validate() error {
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
