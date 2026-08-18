package campaignapp_test

import (
	"context"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// seedExecution creates a second execution under an existing project -- a
// later campaign's rerun of the same service's readiness test.
func seedExecution(t *testing.T, store *fake.Store, name string, projectID int64) (executionID int64) {
	t.Helper()
	e, err := execution.New(name, projectID)
	if err != nil {
		t.Fatalf("execution.New: %v", err)
	}
	executionID, err = store.CreateExecution(context.Background(), e)
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}
	return executionID
}

// fixedClock returns a clock function pinned at seconds (via the shared at()
// helper), so "ended" resolution (which reads Service's injected now) is
// deterministic.
func fixedClock(seconds int) func() time.Time {
	return func() time.Time { return at(seconds) }
}

// report0 is a report with no requested throughput -- unaffected by the
// target-QPS gate (report.ShortOfRequest's own no-target guard).
func report0() report.Report {
	return report.Report{}
}

// reportWithThroughput builds a report requesting/achieving the given
// throughput, for driving ShortOfRequest through the verdict.
func reportWithThroughput(requested, achieved float64) report.Report {
	return report.Report{
		Requested: report.Load{Throughput: requested},
		Achieved:  report.Load{Throughput: achieved},
	}
}

func TestCompare_DefaultBaselineIsMostRecentPriorEndedCampaign(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()

	projectA, execA1 := seedProjectAndExecution(t, store, "service-a", 7)
	mustSaveReport(t, store, execA1, 1, taurus.OutcomePassed, report0())
	older, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "older", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA1}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(older): %v", err)
	}

	// A second, more recent ended campaign for the same tenant -- this is the
	// one the default baseline resolution must pick, not "older".
	_, execA2 := seedProjectAndExecution(t, store, "service-a-2", 7)
	mustSaveReport(t, store, execA2, 2, taurus.OutcomePassed, report0())
	newer, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "newer", TenantID: 7, Window: campaign.Window{Start: at(200), End: at(300)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA2}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(newer): %v", err)
	}

	// The target campaign, still open at "now" = 500 -- both older and newer
	// have ended by then (their windows closed at 100 and 300).
	_, execTarget := seedProjectAndExecution(t, store, "service-a-3", 7)
	mustSaveReport(t, store, execTarget, 3, taurus.OutcomePassed, report0())
	target, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "target", TenantID: 7, Window: campaign.Window{Start: at(400), End: at(1000)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execTarget}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(target): %v", err)
	}

	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(fixedClock(500))

	got, err := svc.Compare(ctx, target, 0)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if !got.HasBaseline || got.BaselineCampaignID != newer {
		t.Fatalf("Compare = %+v, want baseline = newer campaign (%d), not older (%d)", got, newer, older)
	}
}

func TestCompare_ExplicitBaselineOverridesDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()

	projectA, execA1 := seedProjectAndExecution(t, store, "service-a", 7)
	mustSaveReport(t, store, execA1, 1, taurus.OutcomePassed, report0())
	older, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "older", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA1}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(older): %v", err)
	}

	_, execTarget := seedProjectAndExecution(t, store, "service-a-3", 7)
	mustSaveReport(t, store, execTarget, 3, taurus.OutcomePassed, report0())
	target, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "target", TenantID: 7, Window: campaign.Window{Start: at(400), End: at(1000)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execTarget}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(target): %v", err)
	}

	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(fixedClock(500))
	got, err := svc.Compare(ctx, target, older)
	if err != nil {
		t.Fatalf("Compare (explicit baseline): %v", err)
	}
	if !got.HasBaseline || got.BaselineCampaignID != older {
		t.Fatalf("Compare = %+v, want the explicitly-given baseline %d", got, older)
	}
}

// A campaign with no prior ended campaign in its tenant returns an
// explanatory empty comparison -- HasBaseline false, no services, no error.
func TestCompare_NoPriorCampaign_ReturnsExplanatoryEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	mustSaveReport(t, store, execA, 1, taurus.OutcomePassed, report0())
	target, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "target", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}

	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(fixedClock(500))
	got, err := svc.Compare(ctx, target, 0)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.HasBaseline {
		t.Fatalf("Compare = %+v, want HasBaseline false (no prior campaign)", got)
	}
	if len(got.Services) != 0 {
		t.Fatalf("Compare.Services = %+v, want empty when there is no baseline", got.Services)
	}
}

// A campaign whose window has not yet closed is not "ended" and must not be
// picked as a default baseline, even if it started earlier.
func TestCompare_DefaultBaselineExcludesUnendedCampaign(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	mustSaveReport(t, store, execA, 1, taurus.OutcomePassed, report0())
	// Starts before target but its window doesn't close until 900 -- not
	// ended at "now" = 500.
	_, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "still-open", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(900)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(still-open): %v", err)
	}

	_, execTarget := seedProjectAndExecution(t, store, "service-a-3", 7)
	mustSaveReport(t, store, execTarget, 3, taurus.OutcomePassed, report0())
	target, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "target", TenantID: 7, Window: campaign.Window{Start: at(400), End: at(1000)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execTarget}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(target): %v", err)
	}

	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(fixedClock(500))
	got, err := svc.Compare(ctx, target, 0)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got.HasBaseline {
		t.Fatalf("Compare = %+v, want HasBaseline false (only candidate has not ended)", got)
	}
}

// The comparison reuses Verdict for both sides, so the target-QPS gate
// (task 103) applies identically -- a service that passed but fell short of
// its target QPS in the baseline reads as "improved" once it hits target now.
func TestCompare_UsesVerdictSoTargetQPSGateApplies(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()

	// Verdict always reads a designated execution's CURRENT latest report --
	// it has no notion of "as of this campaign's window" -- so a later
	// campaign designates a fresh execution for the same project, exactly as
	// it would in practice (a new readiness run for a repeat campaign).
	// Compare matches the two by ProjectID regardless.
	projectA, execOlder := seedProjectAndExecution(t, store, "service-a", 7)
	// Baseline: passed criteria but only 60% of target -- ShortOfTargetQPS,
	// so Verdict treats it as no-go.
	mustSaveReport(t, store, execOlder, 1, taurus.OutcomePassed, reportWithThroughput(100, 60))
	older, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "older", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execOlder}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(older): %v", err)
	}

	// Target: same project, a fresh execution that comfortably hits target QPS.
	execNewer := seedExecution(t, store, "service-a-rerun", projectA)
	mustSaveReport(t, store, execNewer, 2, taurus.OutcomePassed, reportWithThroughput(100, 99))
	target, err := store.CreateCampaign(ctx, campaign.Campaign{
		Name: "target", TenantID: 7, Window: campaign.Window{Start: at(400), End: at(1000)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execNewer}},
	})
	if err != nil {
		t.Fatalf("CreateCampaign(target): %v", err)
	}

	svc := campaignapp.NewService(store, fake.NewScheduler()).WithNow(fixedClock(500))
	got, err := svc.Compare(ctx, target, older)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if len(got.Services) != 1 || got.Services[0].Status != campaign.ComparisonImproved {
		t.Fatalf("Compare.Services = %+v, want project %d classified improved (was short of target QPS, now hits it)", got.Services, projectA)
	}
}

func TestCompare_MissingCampaignPropagatesNotFound(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())
	if _, err := svc.Compare(context.Background(), 999, 0); err == nil {
		t.Fatal("Compare(missing campaign) = nil error, want ErrNotFound")
	}
}
