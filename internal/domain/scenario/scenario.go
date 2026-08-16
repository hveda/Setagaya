// Package scenario holds the Scenario aggregate: a reusable workload definition
// (a script or JMX file plus data files) that belongs to a Project. It is the
// Taurus "scenarios" concept. Pure domain, no I/O.
package scenario

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/heridotlife/honryu/internal/domain/taurus"
)

// MaxNameLen mirrors the persisted schema (scenario.name VARCHAR(100)).
const MaxNameLen = 100

// Validation errors. Callers compare with errors.Is.
var (
	ErrNameRequired    = errors.New("scenario: name is required")
	ErrNameTooLong     = errors.New("scenario: name exceeds maximum length")
	ErrProjectRequired = errors.New("scenario: a valid project id is required")

	ErrKindUnknown          = errors.New("scenario: unknown kind")
	ErrEngineUnknown        = errors.New("scenario: unknown engine")
	ErrPortableEnginePinned = errors.New("scenario: a portable scenario must not pin an engine")
	ErrNativeEngineRequired = errors.New("scenario: a native scenario must name its engine")

	// ErrEngineNeedsScript and ErrEnginePinned are the two ways an engine
	// selection can be refused; both surface to the caller through the API.
	ErrEngineNeedsScript = errors.New("scenario: engine requires a native script")
	ErrEnginePinned      = errors.New("scenario: scenario is pinned to another engine")
)

// Kind is how a scenario's workload is expressed, which decides how freely it
// moves between engines.
//
// Taurus normalises results across engines but not inputs: the declarative
// `requests:` form runs on most executors, while k6 accepts only a native
// script. So "pick any engine" is true of a portable scenario and false of a
// native one, and the difference has to be visible rather than discovered when
// a run fails in a pod.
type Kind string

const (
	// KindPortable is a scenario expressed as Taurus requests. It runs on any
	// executor that accepts the declarative form.
	KindPortable Kind = "portable"
	// KindNative is a scenario carrying an engine-specific artefact -- a .jmx, a
	// k6 script, a Gatling simulation -- and runs only on that engine.
	KindNative Kind = "native"
)

// Scenario is a test definition owned by a Project.
type Scenario struct {
	ID        int64
	Name      string
	ProjectID int64
	// Kind is how the workload is expressed.
	Kind Kind
	// Engine pins a native scenario to the engine its artefact belongs to. It is
	// empty for portable scenarios.
	Engine      taurus.Executor
	TenantID    *int64
	CreatedBy   string
	UpdatedBy   string
	CreatedTime time.Time
}

// New constructs a portable Scenario: one expressed as Taurus requests, which
// any declarative engine can run. Name is trimmed; ID and CreatedTime are
// assigned by the repository.
func New(name string, projectID int64) (Scenario, error) {
	s := Scenario{Name: strings.TrimSpace(name), ProjectID: projectID, Kind: KindPortable}
	if err := s.Validate(); err != nil {
		return Scenario{}, err
	}
	return s, nil
}

// NewNative constructs a Scenario pinned to one engine, as produced by importing
// an engine-specific artefact such as a Shibuya .jmx.
func NewNative(name string, projectID int64, engine taurus.Executor) (Scenario, error) {
	s := Scenario{
		Name: strings.TrimSpace(name), ProjectID: projectID,
		Kind: KindNative, Engine: engine,
	}
	if err := s.Validate(); err != nil {
		return Scenario{}, err
	}
	return s, nil
}

// Validate checks the Scenario's invariants.
func (s Scenario) Validate() error {
	switch {
	case s.Name == "":
		return ErrNameRequired
	case len(s.Name) > MaxNameLen:
		return ErrNameTooLong
	case s.ProjectID <= 0:
		return ErrProjectRequired
	}

	switch s.Kind {
	case KindPortable:
		if s.Engine != "" {
			return fmt.Errorf("%w: %q", ErrPortableEnginePinned, s.Engine)
		}
	case KindNative:
		if s.Engine == "" {
			return ErrNativeEngineRequired
		}
		if !s.Engine.Known() {
			return fmt.Errorf("%w: %q", ErrEngineUnknown, s.Engine)
		}
	default:
		return fmt.Errorf("%w: %q", ErrKindUnknown, s.Kind)
	}
	return nil
}

// SupportedEngines lists the engines this scenario can run on: its own engine
// when native, every declarative executor when portable. An unrecognised kind
// supports nothing -- see CanRunOn.
func (s Scenario) SupportedEngines() []taurus.Executor {
	switch s.Kind {
	case KindNative:
		return []taurus.Executor{s.Engine}
	case KindPortable:
		return taurus.DeclarativeExecutors()
	default:
		return nil
	}
}

// CanRunOn reports whether the scenario may run on engine, and why not when it
// may not. Callers use this to refuse an engine selection before a config is
// compiled, rather than letting bzt fail inside a pod.
//
// An unrecognised kind is refused rather than assumed portable. Treating it as
// portable would be the dangerous guess: a JMeter-pinned scenario would appear
// to run on any engine and fail only once the pod started.
func (s Scenario) CanRunOn(engine taurus.Executor) error {
	if !engine.Known() {
		return fmt.Errorf("%w: %q", ErrEngineUnknown, engine)
	}
	switch s.Kind {
	case KindNative:
		if engine != s.Engine {
			return fmt.Errorf("%w: scenario carries a %s artefact and cannot run on %s",
				ErrEnginePinned, s.Engine, engine)
		}
		return nil
	case KindPortable:
		if !engine.AcceptsDeclarativeRequests() {
			return fmt.Errorf("%w: %s does not accept Taurus requests, so it needs a native script",
				ErrEngineNeedsScript, engine)
		}
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrKindUnknown, s.Kind)
	}
}
