package campaignapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/domain/campaign"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/reservation"
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

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	projectB, execB := seedProjectAndExecution(t, store, "service-b", 7)
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

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	projectB, execB := seedProjectAndExecution(t, store, "service-b", 7)
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

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
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

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
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

// OtherLoad is the spec's minimum mitigation for the residual freeze-scope
// risk: even though freeze itself can't see cross-service infrastructure
// contention, the verdict at least records what else was active.

func TestVerdict_OtherLoad_IncludesOverlappingReservationExcludesDesignated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	_, execOther := seedProjectAndExecution(t, store, "other", 7)
	_, execOtherHigher := seedProjectAndExecution(t, store, "other-higher-id", 7)

	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The campaign's own designated execution reserving capacity must never
	// show up as "other" load.
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: 7, Cluster: "", EngineCount: 2, Start: at(10), End: at(20), ExecutionID: execA,
	}); err != nil {
		t.Fatalf("CreateReservation designated: %v", err)
	}
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: 7, Cluster: "", EngineCount: 3, Start: at(10), End: at(50), ExecutionID: execOther,
	}); err != nil {
		t.Fatalf("CreateReservation other: %v", err)
	}
	// A second, higher-id execution proves OtherLoad is sorted rather than
	// returned in map-iteration order.
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: 7, Cluster: "", EngineCount: 1, Start: at(60), End: at(70), ExecutionID: execOtherHigher,
	}); err != nil {
		t.Fatalf("CreateReservation other-higher-id: %v", err)
	}

	v, err := svc.Verdict(ctx, created.ID)
	if err != nil {
		t.Fatalf("Verdict: %v", err)
	}
	if len(v.OtherLoad) != 2 || v.OtherLoad[0].ExecutionID != execOther || v.OtherLoad[1].ExecutionID != execOtherHigher {
		t.Fatalf("OtherLoad = %+v, want [execution %d, execution %d] in ascending order", v.OtherLoad, execOther, execOtherHigher)
	}
	first := v.OtherLoad[0]
	if !first.Start.Equal(at(10)) || !first.End.Equal(at(50)) || first.EngineCount != 3 {
		t.Fatalf("OtherLoad[0] = %+v, want start %v end %v engines 3", first, at(10), at(50))
	}
}

func TestVerdict_OtherLoad_IncludesCompletedLaunchExcludesOtherTenant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	// execSame's project belongs to the campaign's own tenant (7); execDiff's
	// project belongs to a different tenant (9) entirely -- tenancy is
	// resolved via the execution's own project, not a TenantID on the
	// execution itself (execution.TenantID is never populated by any real
	// creation path, see campaignapp.Service.otherLoad).
	_, execSame := seedProjectAndExecution(t, store, "same-tenant-project", 7)
	_, execDiff := seedProjectAndExecution(t, store, "other-tenant-project", 9)

	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	store.SetNow(func() time.Time { return at(10) })
	if err := store.StartLaunch(ctx, execSame, "owner", 2, 4); err != nil {
		t.Fatalf("StartLaunch same: %v", err)
	}
	store.SetNow(func() time.Time { return at(30) })
	if err := store.FinishLaunch(ctx, execSame, 4); err != nil {
		t.Fatalf("FinishLaunch same: %v", err)
	}

	store.SetNow(func() time.Time { return at(15) })
	if err := store.StartLaunch(ctx, execDiff, "owner", 5, 6); err != nil {
		t.Fatalf("StartLaunch diff: %v", err)
	}
	store.SetNow(func() time.Time { return at(40) })
	if err := store.FinishLaunch(ctx, execDiff, 6); err != nil {
		t.Fatalf("FinishLaunch diff: %v", err)
	}

	v, err := svc.Verdict(ctx, created.ID)
	if err != nil {
		t.Fatalf("Verdict: %v", err)
	}
	if len(v.OtherLoad) != 1 {
		t.Fatalf("OtherLoad = %+v, want exactly one entry (other-tenant launch excluded)", v.OtherLoad)
	}
	ol := v.OtherLoad[0]
	if ol.ExecutionID != execSame || !ol.Start.Equal(at(10)) || !ol.End.Equal(at(30)) || ol.EngineCount != 2 {
		t.Fatalf("OtherLoad[0] = %+v, want execution %d start %v end %v engines 2", ol, execSame, at(10), at(30))
	}
}

func TestVerdict_OtherLoad_MergesReservationAndLaunchForSameExecution(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := campaignapp.NewService(store, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	tenantID := int64(7)
	execOther, err := store.CreateExecution(ctx, execution.Execution{Name: "other", ProjectID: projectA, TenantID: &tenantID})
	if err != nil {
		t.Fatalf("CreateExecution: %v", err)
	}

	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: tenantID, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Reservation window [30,40) is processed first; the launch below
	// starts earlier and ends later, so merging must widen both edges --
	// not just take whichever source happened to be seen first.
	if _, err := store.CreateReservation(ctx, reservation.Reservation{
		TenantID: tenantID, Cluster: "", EngineCount: 2, Start: at(30), End: at(40), ExecutionID: execOther,
	}); err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	store.SetNow(func() time.Time { return at(10) })
	if err := store.StartLaunch(ctx, execOther, "owner", 5, 6); err != nil {
		t.Fatalf("StartLaunch: %v", err)
	}
	store.SetNow(func() time.Time { return at(60) })
	if err := store.FinishLaunch(ctx, execOther, 6); err != nil {
		t.Fatalf("FinishLaunch: %v", err)
	}

	v, err := svc.Verdict(ctx, created.ID)
	if err != nil {
		t.Fatalf("Verdict: %v", err)
	}
	if len(v.OtherLoad) != 1 {
		t.Fatalf("OtherLoad = %+v, want one merged entry for execution %d", v.OtherLoad, execOther)
	}
	ol := v.OtherLoad[0]
	if ol.ExecutionID != execOther || !ol.Start.Equal(at(10)) || !ol.End.Equal(at(60)) || ol.EngineCount != 5 {
		t.Fatalf("OtherLoad[0] = %+v, want merged start %v end %v engines 5", ol, at(10), at(60))
	}
}

// erroringRepo wraps a *fake.Store and lets a test force one method to fail,
// to prove Verdict propagates a failure from any of its data sources rather
// than swallowing it.
type erroringRepo struct {
	*fake.Store
	reservationsErr  error
	launchHistoryErr error
	listReportsErr   error
	criteriaErr      error
}

func (r *erroringRepo) ReservationsInWindow(ctx context.Context, tenantID int64, cluster string, start, end time.Time) ([]reservation.Reservation, error) {
	if r.reservationsErr != nil {
		return nil, r.reservationsErr
	}
	return r.Store.ReservationsInWindow(ctx, tenantID, cluster, start, end)
}

func (r *erroringRepo) LaunchHistory(ctx context.Context, from, to time.Time) ([]ports.LaunchRecord, error) {
	if r.launchHistoryErr != nil {
		return nil, r.launchHistoryErr
	}
	return r.Store.LaunchHistory(ctx, from, to)
}

func (r *erroringRepo) ListReports(ctx context.Context, executionID int64, limit int) ([]report.Report, error) {
	if r.listReportsErr != nil {
		return nil, r.listReportsErr
	}
	return r.Store.ListReports(ctx, executionID, limit)
}

func (r *erroringRepo) CriteriaFor(ctx context.Context, executionID int64) ([]string, error) {
	if r.criteriaErr != nil {
		return nil, r.criteriaErr
	}
	return r.Store.CriteriaFor(ctx, executionID)
}

func TestVerdict_ServiceReportLookupErrorPropagates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	repo := &erroringRepo{Store: store, listReportsErr: errors.New("boom")}
	svc := campaignapp.NewService(repo, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Verdict(ctx, created.ID); err == nil {
		t.Fatal("Verdict = nil error, want the ListReports failure to propagate")
	}
}

func TestVerdict_FailingCriteriaLookupErrorPropagates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	repo := &erroringRepo{Store: store, criteriaErr: errors.New("boom")}
	svc := campaignapp.NewService(repo, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	mustSaveReport(t, store, execA, 1, taurus.OutcomeFailed, report.Report{ErrorRate: 0.9})
	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Verdict(ctx, created.ID); err == nil {
		t.Fatal("Verdict = nil error, want the CriteriaFor failure to propagate")
	}
}

func TestVerdict_OtherLoad_ReservationsErrorPropagates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	repo := &erroringRepo{Store: store, reservationsErr: errors.New("boom")}
	svc := campaignapp.NewService(repo, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Verdict(ctx, created.ID); err == nil {
		t.Fatal("Verdict = nil error, want the ReservationsInWindow failure to propagate")
	}
}

func TestVerdict_OtherLoad_LaunchHistoryErrorPropagates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	repo := &erroringRepo{Store: store, launchHistoryErr: errors.New("boom")}
	svc := campaignapp.NewService(repo, fake.NewScheduler())

	projectA, execA := seedProjectAndExecution(t, store, "service-a", 7)
	created, err := svc.Create(ctx, campaign.Campaign{
		Name: "c", TenantID: 7, Window: campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{{ProjectID: projectA, ExecutionID: execA}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := svc.Verdict(ctx, created.ID); err == nil {
		t.Fatal("Verdict = nil error, want the LaunchHistory failure to propagate")
	}
}
