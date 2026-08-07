package campaignapp_test

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func mustSaveReport(t *testing.T, store *fake.Store, executionID, runID int64, outcome taurus.Outcome, r report.Report) {
	t.Helper()
	r.ExecutionID = executionID
	r.RunID = runID
	r.Outcome = outcome
	if err := store.SaveReport(context.Background(), r); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}
}

func TestVerdict_AllServicesPassed_OverallGo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a")
	projectB, execB := seedProjectAndExecution(t, store, "service-b")
	mustSaveReport(t, store, execA, 1, taurus.OutcomePassed, report.Report{})
	mustSaveReport(t, store, execB, 2, taurus.OutcomePassed, report.Report{})

	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{
			{ProjectID: projectA, ExecutionID: execA},
			{ProjectID: projectB, ExecutionID: execB},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	v, err := svc.Verdict(ctx, created.ID)
	if err != nil {
		t.Fatalf("Verdict: %v", err)
	}
	if !v.Go {
		t.Fatalf("Verdict.Go = false, want true (both services passed): %+v", v)
	}
	if len(v.Services) != 2 {
		t.Fatalf("Verdict.Services = %+v, want 2", v.Services)
	}
	for _, sv := range v.Services {
		if !sv.HasReport || sv.Outcome != taurus.OutcomePassed {
			t.Errorf("service %d verdict = %+v, want HasReport:true Outcome:passed", sv.ExecutionID, sv)
		}
	}
}

// The whole reason task 65 needed real criteria persistence (task
// "prerequisite" commit): a failed service's verdict must name exactly
// which configured criteria triggered against its latest report.
func TestVerdict_OneServiceFailed_NamesFailingCriteriaAndOverallNoGo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a")
	projectB, execB := seedProjectAndExecution(t, store, "service-b")
	if err := store.SetExecutionCriteria(ctx, execB, []string{"failures>10%"}); err != nil {
		t.Fatalf("SetExecutionCriteria: %v", err)
	}
	mustSaveReport(t, store, execA, 1, taurus.OutcomePassed, report.Report{})
	mustSaveReport(t, store, execB, 2, taurus.OutcomeFailed, report.Report{ErrorRate: 0.20})

	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{
			{ProjectID: projectA, ExecutionID: execA},
			{ProjectID: projectB, ExecutionID: execB},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	v, err := svc.Verdict(ctx, created.ID)
	if err != nil {
		t.Fatalf("Verdict: %v", err)
	}
	if v.Go {
		t.Fatal("Verdict.Go = true, want false -- service-b failed")
	}
	var failed *campaignapp.ServiceVerdict
	for i := range v.Services {
		if v.Services[i].ExecutionID == execB {
			failed = &v.Services[i]
		}
	}
	if failed == nil {
		t.Fatal("no verdict entry for the failed service")
	}
	if len(failed.FailingCriteria) != 1 || failed.FailingCriteria[0].Criterion != "failures>10%" {
		t.Fatalf("failed service FailingCriteria = %+v, want [failures>10%%]", failed.FailingCriteria)
	}
}

// A service with no report at all (never run, or still mid-run) can never
// contribute a go -- distinct from an explicit failure, but equally
// disqualifying for the overall verdict.
func TestVerdict_ServiceWithNoReportYet_OverallNoGo(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a")
	// No report ever saved for execA.

	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	v, err := svc.Verdict(ctx, created.ID)
	if err != nil {
		t.Fatalf("Verdict: %v", err)
	}
	if v.Go {
		t.Fatal("Verdict.Go = true, want false -- the service has no report yet")
	}
	if len(v.Services) != 1 || v.Services[0].HasReport {
		t.Fatalf("Verdict.Services = %+v, want one entry with HasReport:false", v.Services)
	}
}

// An aborted or errored run is not a criteria failure -- FailingCriteria is
// only ever populated for taurus.OutcomeFailed, never guessed at for a run
// that didn't produce a clean pass/fail signal at all.
func TestVerdict_AbortedOutcome_NoFailingCriteriaNamed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a")
	if err := store.SetExecutionCriteria(ctx, execA, []string{"failures>10%"}); err != nil {
		t.Fatalf("SetExecutionCriteria: %v", err)
	}
	mustSaveReport(t, store, execA, 1, taurus.OutcomeAborted, report.Report{ErrorRate: 0.99})

	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	v, err := svc.Verdict(ctx, created.ID)
	if err != nil {
		t.Fatalf("Verdict: %v", err)
	}
	if v.Go {
		t.Fatal("Verdict.Go = true, want false -- aborted is not a pass")
	}
	if len(v.Services[0].FailingCriteria) != 0 {
		t.Fatalf("FailingCriteria for an aborted run = %+v, want none", v.Services[0].FailingCriteria)
	}
}

func TestVerdict_MissingCampaignPropagatesNotFound(t *testing.T) {
	t.Parallel()
	svc := campaignapp.NewService(fake.NewStore(), fake.NewScheduler())
	if _, err := svc.Verdict(context.Background(), 999); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Verdict(missing campaign) = %v, want ErrNotFound", err)
	}
}
