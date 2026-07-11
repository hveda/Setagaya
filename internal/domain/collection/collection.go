// Package collection holds the Collection aggregate: the execution unit that
// groups plans to run together. Pure domain, no I/O.
package collection

import (
	"errors"
	"strings"
	"time"
)

// MaxNameLen mirrors the persisted schema (collection.name VARCHAR(100)).
const MaxNameLen = 100

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired    = errors.New("collection: name is required")
	ErrNameTooLong     = errors.New("collection: name exceeds maximum length")
	ErrProjectRequired = errors.New("collection: a valid project id is required")
)

// Collection is a group of plans executed together against a Project.
type Collection struct {
	ID          int64
	Name        string
	ProjectID   int64
	CSVSplit    bool
	TenantID    *int64
	CreatedBy   string
	UpdatedBy   string
	CreatedTime time.Time
}

// New constructs and validates a Collection. Name is trimmed; ID and
// CreatedTime are assigned by the repository.
func New(name string, projectID int64) (Collection, error) {
	c := Collection{Name: strings.TrimSpace(name), ProjectID: projectID}
	if err := c.Validate(); err != nil {
		return Collection{}, err
	}
	return c, nil
}

// Validate checks the Collection's invariants.
func (c Collection) Validate() error {
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
