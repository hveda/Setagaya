// Package project holds the Project aggregate — a pure domain type with no I/O
// dependencies. A Project is the top of the domain hierarchy:
// Project → Execution → Scenario → ExecutionScenario.
package project

import (
	"errors"
	"strings"
	"time"
)

// Field limits mirror the persisted schema (project table).
const (
	MaxNameLen  = 100
	MaxOwnerLen = 50
	MaxSIDLen   = 25
)

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired  = errors.New("project: name is required")
	ErrNameTooLong   = errors.New("project: name exceeds maximum length")
	ErrOwnerRequired = errors.New("project: owner is required")
	ErrOwnerTooLong  = errors.New("project: owner exceeds maximum length")
	ErrSIDInvalid    = errors.New("project: sid must be numeric")
	ErrSIDTooLong    = errors.New("project: sid exceeds maximum length")
)

// Project is a load-testing project owned by a person or group.
type Project struct {
	ID          int64
	Name        string
	Owner       string
	SID         string
	TenantID    *int64
	CreatedBy   string
	UpdatedBy   string
	CreatedTime time.Time
}

// New constructs and validates a Project from user input. Name and owner are
// trimmed of surrounding whitespace. ID and CreatedTime are assigned by the
// repository on persistence, not here.
func New(name, owner, sid string) (Project, error) {
	p := Project{
		Name:  strings.TrimSpace(name),
		Owner: strings.TrimSpace(owner),
		SID:   strings.TrimSpace(sid),
	}
	if err := p.Validate(); err != nil {
		return Project{}, err
	}
	return p, nil
}

// Validate checks the Project's invariants.
func (p Project) Validate() error {
	switch {
	case p.Name == "":
		return ErrNameRequired
	case len(p.Name) > MaxNameLen:
		return ErrNameTooLong
	case p.Owner == "":
		return ErrOwnerRequired
	case len(p.Owner) > MaxOwnerLen:
		return ErrOwnerTooLong
	}
	if p.SID != "" {
		if len(p.SID) > MaxSIDLen {
			return ErrSIDTooLong
		}
		if !isDigits(p.SID) {
			return ErrSIDInvalid
		}
	}
	return nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
