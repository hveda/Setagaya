package fake_test

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/report"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/internal/ports/reportstoretest"
)

func TestFakeReportStore_Contract(t *testing.T) {
	reportstoretest.Run(t, func(*testing.T) ports.ReportStore {
		return fake.NewReportStore()
	})
}

// The fake can be made to fail, so a use-case's handling of a store outage is
// testable without one.
func TestFakeReportStore_SaveErr(t *testing.T) {
	t.Parallel()
	s := fake.NewReportStore()
	s.SaveErr = errors.New("store down")

	err := s.SaveReport(context.Background(), report.Report{
		ExecutionID: 1, RunID: 1, Outcome: taurus.OutcomePassed,
	})
	if !errors.Is(err, s.SaveErr) {
		t.Errorf("SaveReport = %v, want the injected error", err)
	}
	if _, err := s.GetReport(context.Background(), 1); err == nil {
		t.Error("a failed save still stored the report")
	}
}
