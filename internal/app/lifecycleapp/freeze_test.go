package lifecycleapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/execution"
)

// withCampaigns wires e.svc with a real campaignapp.Service over the same
// store/scheduler, so freeze gating in Trigger actually engages -- setup's
// executions are otherwise never registered in any campaign.
func withCampaigns(e *env) *campaignapp.Service {
	campaigns := campaignapp.NewService(e.store, e.sched)
	e.svc = lifecycleapp.NewService(e.store, e.sched, e.obj, lifecycleapp.StaticImage(image)).WithFreeze(campaigns)
	return campaigns
}

func createCampaign(t *testing.T, campaigns *campaignapp.Service, tenantID, projectID, designatedExecutionID int64, start, end time.Time) campaign.Campaign {
	t.Helper()
	c, err := campaigns.Create(context.Background(), campaign.Campaign{
		Name: "Supersale 11.11", TenantID: tenantID,
		Window:   campaign.Window{Start: start, End: end},
		Services: []campaign.Service{{ProjectID: projectID, ExecutionID: designatedExecutionID}},
	})
	if err != nil {
		t.Fatalf("Create campaign: %v", err)
	}
	return c
}

func TestTrigger_RejectsWhenFrozenByActiveCampaign(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	campaigns := withCampaigns(e)

	// A second execution under the same project is the campaign's
	// designated readiness test; e.executionID is not.
	designated, err := execution.New("readiness", e.projectID)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	designatedID, err := e.store.CreateExecution(ctx, designated)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	createCampaign(t, campaigns, 7, e.projectID, designatedID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); !errors.Is(err, lifecycleapp.ErrCampaignFrozen) {
		t.Fatalf("Trigger (frozen) = %v, want ErrCampaignFrozen", err)
	}
	if _, running, _ := e.store.CurrentRun(ctx, e.executionID); running {
		t.Error("a run started despite being rejected for freeze -- no pod should have been marked running")
	}
}

// The whole point of freeze exempting the designated execution: the
// readiness test itself must be able to run during the very window it
// exists to be tested in.
func TestTrigger_AllowsTheDesignatedExecutionDuringFreeze(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	campaigns := withCampaigns(e)
	createCampaign(t, campaigns, 7, e.projectID, e.executionID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger (designated execution) = %v, want nil", err)
	}
}

func TestTrigger_AllowsWhenNoActiveCampaignCoversTheProject(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2)
	ctx := context.Background()
	campaigns := withCampaigns(e)

	other, err := execution.New("unrelated", e.projectID)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	otherID, err := e.store.CreateExecution(ctx, other)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	// Campaign's window is not yet active.
	createCampaign(t, campaigns, 7, e.projectID, otherID, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour))

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger (campaign not yet active) = %v, want nil", err)
	}
}

// Freeze is opt-in, exactly like Quota: an execution never registered in any
// campaign, with no Freeze hook wired at all, must trigger exactly as it
// always has.
func TestTrigger_SkipsFreezeWhenNoneWired(t *testing.T) {
	t.Parallel()
	e := setup(t, false, 2) // setup wires no Freeze at all
	ctx := context.Background()

	if err := e.svc.Deploy(ctx, e.executionID); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if err := e.svc.Trigger(ctx, e.executionID); err != nil {
		t.Fatalf("Trigger with no freeze wired must succeed: %v", err)
	}
}
