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

// seedProjectAndExecution creates a project and an execution under it,
// returning both ids.
func seedProjectAndExecution(t *testing.T, store *fake.Store, name string) (projectID, executionID int64) {
	t.Helper()
	ctx := context.Background()
	p, err := project.New(name, "honryu", "")
	if err != nil {
		t.Fatalf("project.New: %v", err)
	}
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
	svc := campaignapp.NewService(store)

	projectA, execA := seedProjectAndExecution(t, store, "service-a")
	projectB, execB := seedProjectAndExecution(t, store, "service-b")

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
	svc := campaignapp.NewService(store)

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
	svc := campaignapp.NewService(store)

	_, execA := seedProjectAndExecution(t, store, "service-a")
	projectB, _ := seedProjectAndExecution(t, store, "service-b")

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
	svc := campaignapp.NewService(store)

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
	svc := campaignapp.NewService(fake.NewStore())
	if _, err := svc.Get(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
}

func TestList_ScopesByTenant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store)
	projectID, execID := seedProjectAndExecution(t, store, "service-a")

	base := campaign.Campaign{
		Name:     "c",
		Window:   campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectID, ExecutionID: execID}},
	}
	c7 := base
	c7.TenantID = 7
	c9 := base
	c9.TenantID = 9

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

func TestAbort_SetsAbortedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	now := at(500)
	svc := campaignapp.NewService(store).WithNow(func() time.Time { return now })

	projectID, execID := seedProjectAndExecution(t, store, "service-a")
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
