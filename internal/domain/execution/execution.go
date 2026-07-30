// Package execution holds the Execution aggregate: the runnable unit that
// groups scenarios to run together against a Project. It is the Taurus
// "execution" concept. Pure domain, no I/O.
package execution

import (
	"errors"
	"strings"
	"time"
)

// MaxNameLen mirrors the persisted schema (execution.name VARCHAR(100)).
const MaxNameLen = 100

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired    = errors.New("execution: name is required")
	ErrNameTooLong     = errors.New("execution: name exceeds maximum length")
	ErrProjectRequired = errors.New("execution: a valid project id is required")
)

// Execution is a group of plans executed together against a Project.
type Execution struct {
	ID          int64
	Name        string
	ProjectID   int64
	CSVSplit    bool
	TenantID    *int64
	CreatedBy   string
	UpdatedBy   string
	CreatedTime time.Time
}

// New constructs and validates a Execution. Name is trimmed; ID and
// CreatedTime are assigned by the repository.
func New(name string, projectID int64) (Execution, error) {
	c := Execution{Name: strings.TrimSpace(name), ProjectID: projectID}
	if err := c.Validate(); err != nil {
		return Execution{}, err
	}
	return c, nil
}

// Validate checks the Execution's invariants.
func (c Execution) Validate() error {
	switch {
	case c.Name == "":
		return ErrNameRequired
	case len(c.Name) > MaxNameLen:
		return ErrNameTooLong
	case c.ProjectID <= 0:
		return ErrProjectRequired
	}
	return nil
}
