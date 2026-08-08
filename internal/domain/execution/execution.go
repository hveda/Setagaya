// Package execution holds the Execution aggregate: the runnable unit that
// groups scenarios to run together against a Project. It is the Taurus
// "execution" concept. Pure domain, no I/O.
package execution

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// MaxNameLen mirrors the persisted schema (execution.name VARCHAR(100)).
const MaxNameLen = 100

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired    = errors.New("execution: name is required")
	ErrNameTooLong     = errors.New("execution: name exceeds maximum length")
	ErrProjectRequired = errors.New("execution: a valid project id is required")
	ErrEngineUnknown   = errors.New("execution: unknown engine")
	ErrKindUnknown     = errors.New("execution: unknown kind")
)

// Kind distinguishes what an execution is for.
//
// Empty is treated as KindNormal (the same "empty is the sentinel default"
// convention Engine already uses): an execution row persisted before Kind
// existed decodes to the Go zero value and must keep behaving exactly as it
// always did, with no backfill migration required. New always assigns the
// canonical KindNormal explicitly, so only pre-existing rows ever carry "".
type Kind string

const (
	// KindNormal is an ordinary execution: what every execution was before
	// Kind existed, and what New assigns by default.
	KindNormal Kind = "normal"
	// KindCalibrateEngine is an engine-capacity search (Phase 7): it drives
	// a sequence of single-pod runs at increasing QPS to find where the
	// engine or the target saturates, rather than producing a verdict of
	// its own.
	KindCalibrateEngine Kind = "calibrate_engine"
)

// knownKinds records the kinds Honryu recognises, empty excluded -- mirrors
// taurus.Executor's declarativeSupport map shape.
var knownKinds = map[Kind]bool{
	KindNormal:          true,
	KindCalibrateEngine: true,
}

// Known reports whether k is a Kind Honryu recognises.
func (k Kind) Known() bool {
	return knownKinds[k]
}

// Execution is a group of scenarios executed together against a Project.
type Execution struct {
	ID        int64
	Name      string
	ProjectID int64
	// Engine is the load-test engine this execution runs on. Empty means the
	// deployment's configured default, so an execution created before an
	// operator offered a choice keeps working.
	Engine taurus.Executor
	// Kind distinguishes an ordinary execution from a CalibrateEngine one.
	// See Kind's own doc for the empty-means-Normal convention.
	Kind        Kind
	CSVSplit    bool
	TenantID    *int64
	CreatedBy   string
	UpdatedBy   string
	CreatedTime time.Time
}

// New constructs and validates a Execution. Name is trimmed; ID and
// CreatedTime are assigned by the repository. Kind defaults to KindNormal.
func New(name string, projectID int64) (Execution, error) {
	c := Execution{Name: strings.TrimSpace(name), ProjectID: projectID, Kind: KindNormal}
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
	if c.Engine != "" && !c.Engine.Known() {
		return fmt.Errorf("%w: %q", ErrEngineUnknown, c.Engine)
	}
	if c.Kind != "" && !c.Kind.Known() {
		return fmt.Errorf("%w: %q", ErrKindUnknown, c.Kind)
	}
	return nil
}
