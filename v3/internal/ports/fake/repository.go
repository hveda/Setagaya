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

	"github.com/heridotlife/Setagaya/v3/internal/domain/collection"
	"github.com/heridotlife/Setagaya/v3/internal/domain/execution"
	"github.com/heridotlife/Setagaya/v3/internal/domain/plan"
	"github.com/heridotlife/Setagaya/v3/internal/domain/project"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// Store is an in-memory implementation of every repository port.
type Store struct {
	mu  sync.Mutex
	now func() time.Time

	projectSeq int64
	projects   map[int64]project.Project

	planSeq  int64
	plans    map[int64]plan.Plan
	planTest map[int64]string              // planID -> JMX filename
	planData map[int64]map[string]struct{} // planID -> data filenames

	collSeq     int64
	collections map[int64]collection.Collection
	collData    map[int64]map[string]struct{}       // collectionID -> data filenames
	exec        map[int64][]execution.ExecutionPlan // collectionID -> execution plans

	runSeq     int64
	currentRun map[int64]int64               // collectionID -> active runID
	runHistory map[int64]*ports.RunRecord    // runID -> history row
	running    map[int64]map[int64]time.Time // collectionID -> planID -> startedTime

	deployContext string
	openLaunch    map[int64]*ports.LaunchRecord // collectionID -> open launch
	launchHistory []*ports.LaunchRecord
}

// NewStore returns an empty in-memory Store.
func NewStore() *Store {
	return &Store{
		now:           time.Now,
		projects:      make(map[int64]project.Project),
		plans:         make(map[int64]plan.Plan),
		planTest:      make(map[int64]string),
		planData:      make(map[int64]map[string]struct{}),
		collections:   make(map[int64]collection.Collection),
		collData:      make(map[int64]map[string]struct{}),
		exec:          make(map[int64][]execution.ExecutionPlan),
		currentRun:    make(map[int64]int64),
		runHistory:    make(map[int64]*ports.RunRecord),
		running:       make(map[int64]map[int64]time.Time),
		deployContext: "default",
		openLaunch:    make(map[int64]*ports.LaunchRecord),
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
	_ ports.ProjectRepository    = (*Store)(nil)
	_ ports.PlanRepository       = (*Store)(nil)
	_ ports.CollectionRepository = (*Store)(nil)
	_ ports.RunRepository        = (*Store)(nil)
	_ ports.UsageRepository      = (*Store)(nil)
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

func (s *Store) CreatePlan(_ context.Context, p plan.Plan) (int64, error) {
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

func (s *Store) GetPlan(_ context.Context, id int64) (plan.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.plans[id]
	if !ok {
		return plan.Plan{}, ports.ErrNotFound
	}
	return p, nil
}

func (s *Store) ListPlansByProject(_ context.Context, projectID int64) ([]plan.Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []plan.Plan{}
	for _, p := range s.plans {
		if p.ProjectID == projectID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Store) DeletePlan(_ context.Context, id int64) error {
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

func (s *Store) AddPlanFile(_ context.Context, planID int64, filename string, isTest bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if isTest {
		if _, exists := s.planTest[planID]; exists {
			return ports.ErrFileExists
		}
		s.planTest[planID] = filename
		return nil
	}
	files := s.planData[planID]
	if files == nil {
		files = make(map[string]struct{})
		s.planData[planID] = files
	}
	if _, exists := files[filename]; exists {
		return ports.ErrFileExists
	}
	files[filename] = struct{}{}
	return nil
}

func (s *Store) PlanFilesFor(_ context.Context, planID int64) (ports.PlanFiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pf := ports.PlanFiles{TestFile: s.planTest[planID], Data: keys(s.planData[planID])}
	return pf, nil
}

func (s *Store) DeletePlanFile(_ context.Context, planID int64, filename string, isTest bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if isTest {
		if s.planTest[planID] != filename {
			return ports.ErrNotFound
		}
		delete(s.planTest, planID)
		return nil
	}
	files := s.planData[planID]
	if _, ok := files[filename]; !ok {
		return ports.ErrNotFound
	}
	delete(files, filename)
	return nil
}

func (s *Store) PlanInUse(_ context.Context, planID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, plans := range s.exec {
		for _, ep := range plans {
			if ep.PlanID == planID {
				return true, nil
			}
		}
	}
	return false, nil
}

// --- Collections ------------------------------------------------------------

func (s *Store) CreateCollection(_ context.Context, c collection.Collection) (int64, error) {
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

func (s *Store) GetCollection(_ context.Context, id int64) (collection.Collection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.collections[id]
	if !ok {
		return collection.Collection{}, ports.ErrNotFound
	}
	return c, nil
}

func (s *Store) ListCollectionsByProject(_ context.Context, projectID int64) ([]collection.Collection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []collection.Collection{}
	for _, c := range s.collections {
		if c.ProjectID == projectID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *Store) DeleteCollection(_ context.Context, id int64) error {
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

func (s *Store) AddCollectionFile(_ context.Context, collectionID int64, filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := s.collData[collectionID]
	if files == nil {
		files = make(map[string]struct{})
		s.collData[collectionID] = files
	}
	if _, exists := files[filename]; exists {
		return ports.ErrFileExists
	}
	files[filename] = struct{}{}
	return nil
}

func (s *Store) CollectionFilesFor(_ context.Context, collectionID int64) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return keys(s.collData[collectionID]), nil
}

func (s *Store) DeleteCollectionFile(_ context.Context, collectionID int64, filename string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := s.collData[collectionID]
	if _, ok := files[filename]; !ok {
		return ports.ErrNotFound
	}
	delete(files, filename)
	return nil
}

func (s *Store) StoreExecutionCollection(_ context.Context, collectionID int64, csvSplit bool, plans []execution.ExecutionPlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.collections[collectionID]
	if !ok {
		return ports.ErrNotFound
	}
	// The persisted schema (collection_plan) does not store the plan name, so
	// the fake drops it too to stay behaviourally identical to MySQL.
	stored := make([]execution.ExecutionPlan, len(plans))
	for i, ep := range plans {
		ep.Name = ""
		stored[i] = ep
	}
	s.exec[collectionID] = stored
	c.CSVSplit = csvSplit
	s.collections[collectionID] = c
	return nil
}

func (s *Store) ExecutionPlansFor(_ context.Context, collectionID int64) ([]execution.ExecutionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]execution.ExecutionPlan(nil), s.exec[collectionID]...), nil
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
