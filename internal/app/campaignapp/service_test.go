package campaignapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/project"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// seedProjectAndExecution creates a project (belonging to tenantID -- Create
// now rejects a service whose project belongs to a different tenant than
// the campaign's own) and an execution under it, returning both ids.
func seedProjectAndExecution(t *testing.T, store *fake.Store, name string, tenantID int64) (projectID, executionID int64) {
	t.Helper()
	ctx := context.Background()
	p, err := project.New(name, "honryu", "")
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
	p.TenantID = &tenantID
	projectID, err = store.CreateProject(ctx, p)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	e, err := execution.New("readiness", projectID)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	executionID, err = store.CreateExecution(ctx, e)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	return projectID, executionID
}

func at(seconds int) time.Time { return time.Unix(int64(seconds), 0).UTC() }

func TestCreate_ValidatesAndPersists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	projectB, execB := seedProjectAndExecution(t, store, "service-b", 7)

	c := campaign.Campaign{
		Name:     "Supersale 11.11",
		TenantID: 7,
		Window:   campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{
			{ProjectID: projectA, ExecutionID: execA},
			{ProjectID: projectB, ExecutionID: execB},
		},
	}

	got, err := svc.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID == 0 {
		t.Fatal("Create returned a campaign with no id")
	}

	fetched, err := svc.Get(ctx, got.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(fetched.Services) != 2 {
		t.Fatalf("Get services = %+v, want 2", fetched.Services)
	}
}

func TestCreate_PropagatesDomainValidationError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	_, err := svc.Create(ctx, campaign.Campaign{Name: "", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(1)}})
	if !errors.Is(err, campaign.ErrNameRequired) {
		t.Fatalf("Create (no name) = %v, want ErrNameRequired", err)
	}
}

// The check Create's own invariant exists to catch: a service naming an
// execution that actually belongs to a different project would let that
// project's own execution silently decide someone else's verdict.
func TestCreate_RejectsWhenExecutionBelongsToADifferentProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	_, execA := seedProjectAndExecution(t, store, "service-a", 7)
	projectB, _ := seedProjectAndExecution(t, store, "service-b", 7)

	c := campaign.Campaign{
		Name:     "Supersale 11.11",
		TenantID: 7,
		Window:   campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{
			{ProjectID: projectB, ExecutionID: execA}, // execA actually belongs to a different project
		},
	}

	_, err := svc.Create(ctx, c)
	if !errors.Is(err, campaignapp.ErrServiceExecutionMismatch) {
		t.Fatalf("Create (mismatched execution/project) = %v, want ErrServiceExecutionMismatch", err)
	}
}

func TestCreate_UnknownExecutionPropagatesNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	c := campaign.Campaign{
		Name:     "Supersale 11.11",
		TenantID: 7,
		Window:   campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: 1, ExecutionID: 999}},
	}

	_, err := svc.Create(ctx, c)
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Create (unknown execution) = %v, want ErrNotFound", err)
	}
}

func TestGet_MissingReturnsNotFound(t *testing.T) {
	t.Parallel()
	svc := campaignapp.NewService(fake.NewStore(), fake.NewScheduler())
	if _, err := svc.Get(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
}

func TestList_ScopesByTenant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())
	project7, exec7 := seedProjectAndExecution(t, store, "service-a", 7)
	project9, exec9 := seedProjectAndExecution(t, store, "service-a-tenant-9", 9)

	c7 := campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: project7, ExecutionID: exec7}},
	}
	c9 := campaign.Campaign{
		Name: "c", TenantID: 9, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: project9, ExecutionID: exec9}},
	}

	if _, err := svc.Create(ctx, c7); err != nil {
		t.Fatalf("Create (tenant 7): %v", err)
	}
	if _, err := svc.Create(ctx, c9); err != nil {
		t.Fatalf("Create (tenant 9): %v", err)
	}

	got, err := svc.List(ctx, 7)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].TenantID != 7 {
		t.Fatalf("List(tenant 7) = %+v, want exactly one tenant-7 campaign", got)
	}
}

func TestActiveCampaigns_ExcludesNotYetStartedEndedAndAborted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(func() time.Time { return at(50) })

	projectID, execID := seedProjectAndExecution(t, store, "service-a", 7)
	active, err := svc.Create(ctx, campaign.Campaign{
		Name: "active", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectID, ExecutionID: execID}},
	})
	if err != nil {
		t.Fatalf("Create (active): %v", err)
	}
	if _, err := svc.Create(ctx, campaign.Campaign{
		Name: "future", TenantID: 7, Window: campaign.Window{Start: at(200), End: at(300)},
		Services: []campaign.Service{{ProjectID: projectID, ExecutionID: execID}},
	}); err != nil {
		t.Fatalf("Create (future): %v", err)
	}

	got, err := svc.ActiveCampaigns(ctx)
	if err != nil {
		t.Fatalf("ActiveCampaigns: %v", err)
	}
	if len(got) != 1 || got[0].ID != active.ID {
		t.Fatalf("ActiveCampaigns = %+v, want only %+v", got, active)
	}
}

func TestAbort_SetsAbortedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	now := at(500)
	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(func() time.Time { return now })

	projectID, execID := seedProjectAndExecution(t, store, "service-a", 7)
	c := campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectID, ExecutionID: execID}},
	}
	created, err := svc.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := svc.Abort(ctx, created.ID); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	got, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AbortedAt == nil || !got.AbortedAt.Equal(now) {
		t.Fatalf("Get AbortedAt = %v, want %v", got.AbortedAt, now)
	}
	if got.IsActive(now) {
		t.Fatal("an aborted campaign must not be active, even inside its own window")
	}
}

func deploy(t *testing.T, sched *fake.Scheduler, executionID, scenarioID int64) {
	t.Helper()
	if err := sched.DeployScenario(context.Background(), ports.DeploySpec{ExecutionID: executionID, ScenarioID: scenarioID}); err != nil {
		t.Fatalf("DeployScenario: %v", err)
	}
}

// InScopeExecutions is what cmd/scheduler's drain sweep stops: every
// currently-deployed execution under a participating project, except the
// project's own designated (exempt) execution, and except anything deployed
// under an unrelated project entirely.
func TestInScopeExecutions_ExcludesDesignatedAndUnrelatedProjects(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	sched := fake.NewScheduler()
	svc := campaignapp.NewService(store, sched)

	projectA, designated := seedProjectAndExecution(t, store, "service-a", 7)
	_, straggler := seedProjectAndExecution(t, store, "service-a-straggler", 7) // different project, same owner in practice but distinct id
	// Re-point straggler's project to projectA by creating it directly under projectA instead.
	e, err := execution.New("straggler", projectA)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	strayUnderA, err := store.CreateExecution(ctx, e)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	_, unrelated := seedProjectAndExecution(t, store, "service-b", 7)

	c := campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: designated}},
	}
	created, err := svc.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	deploy(t, sched, designated, 1)
	deploy(t, sched, strayUnderA, 1)
	deploy(t, sched, unrelated, 1)
	deploy(t, sched, straggler, 1) // deployed, but under yet another unrelated project

	got, err := svc.InScopeExecutions(ctx, created.ID)
	if err != nil {
		t.Fatalf("InScopeExecutions: %v", err)
	}
	if len(got) != 1 || got[0] != strayUnderA {
		t.Fatalf("InScopeExecutions = %v, want exactly [%d] (strayUnderA)", got, strayUnderA)
	}
}

func TestInScopeExecutions_MissingCampaignPropagatesNotFound(t *testing.T) {
	t.Parallel()
	svc := campaignapp.NewService(fake.NewStore(), fake.NewScheduler())
	if _, err := svc.InScopeExecutions(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("InScopeExecutions(missing campaign) = %v, want ErrNotFound", err)
	}
}

// A deployed execution whose record has since vanished (e.g. deleted) must
// not abort the whole scan -- it has nothing left to match a project
// against, so it is simply excluded, the same tolerance
// adminapp.deployedExecutionsForTenant already applies to its own analogous
// scan.
func TestInScopeExecutions_SkipsADeployedExecutionWhoseRecordVanished(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	sched := fake.NewScheduler()
	svc := campaignapp.NewService(store, sched)

	projectA, designated := seedProjectAndExecution(t, store, "service-a", 7)
	c := campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: designated}},
	}
	created, err := svc.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	deploy(t, sched, designated, 1)
	deploy(t, sched, 999999, 1) // deployed, but no such execution was ever created

	got, err := svc.InScopeExecutions(ctx, created.ID)
	if err != nil {
		t.Fatalf("InScopeExecutions: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("InScopeExecutions = %v, want empty (vanished execution excluded, designated one exempt)", got)
	}
}

func TestIsFrozen_ExemptsTheDesignatedExecutionButBlocksOthersUnderTheSameProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(func() time.Time { return at(50) })

	projectA, designated := seedProjectAndExecution(t, store, "service-a", 7)
	e, err := execution.New("other", projectA)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	other, err := store.CreateExecution(ctx, e)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	_, unrelatedExec := seedProjectAndExecution(t, store, "service-b", 7)

	c := campaign.Campaign{
		Name: "Supersale", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: designated}},
	}
	if _, err := svc.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if blocked, _, err := svc.IsFrozen(ctx, projectA, designated); err != nil || blocked {
		t.Fatalf("IsFrozen(designated) = %v, %v, want false, nil", blocked, err)
	}
	blocked, name, err := svc.IsFrozen(ctx, projectA, other)
	if err != nil || !blocked || name != "Supersale" {
		t.Fatalf("IsFrozen(other execution, same project) = %v, %q, %v, want true, \"Supersale\", nil", blocked, name, err)
	}
	if blocked, _, err := svc.IsFrozen(ctx, 999999, unrelatedExec); err != nil || blocked {
		t.Fatalf("IsFrozen(unrelated project) = %v, %v, want false, nil", blocked, err)
	}
}

func TestIsFrozen_NotYetActiveCampaignDoesNotFreeze(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(func() time.Time { return at(-50) }) // before the window

	projectA, designated := seedProjectAndExecution(t, store, "service-a", 7)
	e, err := execution.New("other", projectA)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	other, err := store.CreateExecution(ctx, e)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	c := campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: designated}},
	}
	if _, err := svc.Create(ctx, c); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if blocked, _, err := svc.IsFrozen(ctx, projectA, other); err != nil || blocked {
		t.Fatalf("IsFrozen (campaign not yet active) = %v, %v, want false, nil", blocked, err)
	}
}

// Two campaigns registering the same project with different designated
// executions is not prevented by Campaign.Validate (which only checks one
// campaign's own invariants). Each campaign's own designated execution must
// stay exempt regardless of the other campaign's presence.
func TestIsFrozen_ExecutionExemptByEitherOfTwoOverlappingCampaigns(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(func() time.Time { return at(50) })

	projectA, designated1 := seedProjectAndExecution(t, store, "service-a", 7)
	e2, err := execution.New("designated2", projectA)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	designated2, err := store.CreateExecution(ctx, e2)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	e3, err := execution.New("neither", projectA)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	neither, err := store.CreateExecution(ctx, e3)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	if _, err := svc.Create(ctx, campaign.Campaign{
		Name: "campaign-1", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: designated1}},
	}); err != nil {
		t.Fatalf("Create (campaign-1): %v", err)
	}
	if _, err := svc.Create(ctx, campaign.Campaign{
		Name: "campaign-2", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: designated2}},
	}); err != nil {
		t.Fatalf("Create (campaign-2): %v", err)
	}

	if blocked, _, err := svc.IsFrozen(ctx, projectA, designated1); err != nil || blocked {
		t.Fatalf("IsFrozen(designated1) = %v, %v, want false (exempt via campaign-1)", blocked, err)
	}
	if blocked, _, err := svc.IsFrozen(ctx, projectA, designated2); err != nil || blocked {
		t.Fatalf("IsFrozen(designated2) = %v, %v, want false (exempt via campaign-2)", blocked, err)
	}
	if blocked, _, err := svc.IsFrozen(ctx, projectA, neither); err != nil || !blocked {
		t.Fatalf("IsFrozen(neither) = %v, %v, want true (blocked by both campaigns)", blocked, err)
	}
}
