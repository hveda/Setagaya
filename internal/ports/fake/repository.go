// Package fake provides in-memory implementations of the repository ports for
// fast, hermetic unit tests of the application layer. A single Store backs all
// aggregates so cross-aggregate rules (e.g. "is this plan used by a
// collection") behave like the real shared database. Behaviour is pinned by the
// conformance suites in internal/ports/repositorytest.
package fake

import (
	"context"
	"sync"
	"time"

	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/domain/project"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/domain/tenant"
	"github.com/heridotlife/Setagaya/internal/ports"
)

// Store is an in-memory implementation of every repository port.
type Store struct {
	mu  sync.Mutex
	now func() time.Time

	projectSeq int64
	projects   map[int64]project.Project

	planSeq  int64
	plans    map[int64]scenario.Scenario
	planTest map[int64]string              // scenarioID -> JMX filename
	planData map[int64]map[string]struct{} // scenarioID -> data filenames

	collSeq     int64
	collections map[int64]execution.Execution
	collData    map[int64]map[string]struct{} // executionID -> data filenames
	exec        map[int64][]loadprofile.Entry // executionID -> execution plans

	runSeq     int64
	currentRun map[int64]int64               // executionID -> active runID
	runHistory map[int64]*ports.RunRecord    // runID -> history row
	running    map[int64]map[int64]time.Time // executionID -> scenarioID -> startedTime

	deployContext string
	openLaunch    map[int64]*ports.LaunchRecord // executionID -> open launch
	launchHistory []*ports.LaunchRecord

	tenantSeq int64
	tenants   map[int64]tenant.Tenant
	grants    []ports.RoleGrant // role assignments, deduped by subject/role/tenant
}

// NewStore returns an empty in-memory Store.
func NewStore() *Store {
	return &Store{
		now:           time.Now,
		projects:      make(map[int64]project.Project),
		plans:         make(map[int64]scenario.Scenario),
		planTest:      make(map[int64]string),
		planData:      make(map[int64]map[string]struct{}),
		collections:   make(map[int64]execution.Execution),
		collData:      make(map[int64]map[string]struct{}),
		exec:          make(map[int64][]loadprofile.Entry),
		currentRun:    make(map[int64]int64),
		runHistory:    make(map[int64]*ports.RunRecord),
		running:       make(map[int64]map[int64]time.Time),
		deployContext: "default",
		openLaunch:    make(map[int64]*ports.LaunchRecord),
		tenants:       make(map[int64]tenant.Tenant),
	}
}

// NewProjectRepository returns a Store viewed as a ProjectRepository.
func NewProjectRepository() *Store { return NewStore() }

// SetNow overrides the clock used for created/started timestamps, for
// deterministic tests.
func (s *Store) SetNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

var (
	_ ports.ProjectRepository        = (*Store)(nil)
	_ ports.ScenarioRepository       = (*Store)(nil)
	_ ports.ExecutionRepository      = (*Store)(nil)
	_ ports.RunRepository            = (*Store)(nil)
	_ ports.UsageRepository          = (*Store)(nil)
	_ ports.TenantRepository         = (*Store)(nil)
	_ ports.RoleAssignmentRepository = (*Store)(nil)
)

// --- Projects ---------------------------------------------------------------

func (s *Store) CreateProject(_ context.Context, p project.Project) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.projectSeq++
	p.ID = s.projectSeq
	if p.CreatedTime.IsZero() {
		p.CreatedTime = s.now()
	}
	s.projects[p.ID] = p
	return p.ID, nil
}

func (s *Store) GetProject(_ context.Context, id int64) (project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.projects[id]
	if !ok {
		return project.Project{}, ports.ErrNotFound
	}
	return p, nil
}

func (s *Store) ListProjectsByOwners(_ context.Context, owners []string) ([]project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := toSet(owners)
	out := []project.Project{}
	for _, p := range s.projects {
		if _, ok := want[p.Owner]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Store) ListAllProjects(_ context.Context) ([]project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]project.Project, 0, len(s.projects))
	for _, p := range s.projects {
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) ListProjectsByTenants(_ context.Context, tenantIDs []int64) ([]project.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := make(map[int64]struct{}, len(tenantIDs))
	for _, id := range tenantIDs {
		want[id] = struct{}{}
	}
	out := []project.Project{}
	for _, p := range s.projects {
		if p.TenantID == nil {
			continue
		}
		if _, ok := want[*p.TenantID]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Store) DeleteProject(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.projects[id]; !ok {
		return ports.ErrNotFound
	}
	delete(s.projects, id)
	return nil
}

// --- Plans ------------------------------------------------------------------

func (s *Store) CreateScenario(_ context.Context, p scenario.Scenario) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.planSeq++
	p.ID = s.planSeq
	if p.CreatedTime.IsZero() {
		p.CreatedTime = s.now()
	}
	s.plans[p.ID] = p
	return p.ID, nil
}

func (s *Store) GetScenario(_ context.Context, id int64) (scenario.Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[id]
	if !ok {
		return scenario.Scenario{}, ports.ErrNotFound
	}
	return p, nil
}

func (s *Store) ListScenariosByProject(_ context.Context, projectID int64) ([]scenario.Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []scenario.Scenario{}
	for _, p := range s.plans {
		if p.ProjectID == projectID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Store) DeleteScenario(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.plans[id]; !ok {
		return ports.ErrNotFound
	}
	delete(s.plans, id)
	delete(s.planTest, id)
	delete(s.planData, id)
	return nil
}

func (s *Store) AddScenarioFile(_ context.Context, scenarioID int64, filename string, isTest bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if isTest {
		if _, exists := s.planTest[scenarioID]; exists {
			return ports.ErrFileExists
		}
		s.planTest[scenarioID] = filename
		return nil
	}
	files := s.planData[scenarioID]
	if files == nil {
		files = make(map[string]struct{})
		s.planData[scenarioID] = files
	}
	if _, exists := files[filename]; exists {
		return ports.ErrFileExists
	}
	files[filename] = struct{}{}
	return nil
}

func (s *Store) ScenarioFilesFor(_ context.Context, scenarioID int64) (ports.ScenarioFiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pf := ports.ScenarioFiles{TestFile: s.planTest[scenarioID], Data: keys(s.planData[scenarioID])}
	return pf, nil
}

func (s *Store) DeleteScenarioFile(_ context.Context, scenarioID int64, filename string, isTest bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if isTest {
		if s.planTest[scenarioID] != filename {
			return ports.ErrNotFound
		}
		delete(s.planTest, scenarioID)
		return nil
	}
	files := s.planData[scenarioID]
	if _, ok := files[filename]; !ok {
		return ports.ErrNotFound
	}
	delete(files, filename)
	return nil
}

func (s *Store) ScenarioInUse(_ context.Context, scenarioID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, plans := range s.exec {
		for _, ep := range plans {
			if ep.ScenarioID == scenarioID {
				return true, nil
			}
		}
	}
	return false, nil
}

// --- Collections ------------------------------------------------------------

func (s *Store) CreateExecution(_ context.Context, c execution.Execution) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.collSeq++
	c.ID = s.collSeq
	if c.CreatedTime.IsZero() {
		c.CreatedTime = s.now()
	}
	s.collections[c.ID] = c
	return c.ID, nil
}

func (s *Store) GetExecution(_ context.Context, id int64) (execution.Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.collections[id]
	if !ok {
		return execution.Execution{}, ports.ErrNotFound
	}
	return c, nil
}

func (s *Store) ListExecutionsByProject(_ context.Context, projectID int64) ([]execution.Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []execution.Execution{}
	for _, c := range s.collections {
		if c.ProjectID == projectID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Store) DeleteExecution(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.collections[id]; !ok {
		return ports.ErrNotFound
	}
	delete(s.collections, id)
	delete(s.collData, id)
	delete(s.exec, id)
	return nil
}

func (s *Store) AddExecutionFile(_ context.Context, executionID int64, filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := s.collData[executionID]
	if files == nil {
		files = make(map[string]struct{})
		s.collData[executionID] = files
	}
	if _, exists := files[filename]; exists {
		return ports.ErrFileExists
	}
	files[filename] = struct{}{}
	return nil
}

func (s *Store) ExecutionFilesFor(_ context.Context, executionID int64) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return keys(s.collData[executionID]), nil
}

func (s *Store) DeleteExecutionFile(_ context.Context, executionID int64, filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := s.collData[executionID]
	if _, ok := files[filename]; !ok {
		return ports.ErrNotFound
	}
	delete(files, filename)
	return nil
}

func (s *Store) StoreLoadProfile(_ context.Context, executionID int64, csvSplit bool, plans []loadprofile.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.collections[executionID]
	if !ok {
		return ports.ErrNotFound
	}
	// The persisted schema (execution_scenario) does not store the plan name, so
	// the fake drops it too to stay behaviourally identical to MySQL.
	stored := make([]loadprofile.Entry, len(plans))
	for i, ep := range plans {
		ep.Name = ""
		stored[i] = ep
	}
	s.exec[executionID] = stored
	c.CSVSplit = csvSplit
	s.collections[executionID] = c
	return nil
}

func (s *Store) LoadProfileFor(_ context.Context, executionID int64) ([]loadprofile.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]loadprofile.Entry(nil), s.exec[executionID]...), nil
}

// --- helpers ----------------------------------------------------------------

func toSet(ss []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		set[s] = struct{}{}
	}
	return set
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
