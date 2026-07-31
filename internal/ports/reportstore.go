package ports

import (
	"context"
	"time"

	"github.com/heridotlife/honryu/internal/domain/report"
)

// ReportStore persists what a run produced.
//
// Reports are kept because they are the evidence a readiness judgement was made
// on. They must outlive the engines that produced them, the metrics backend's
// retention, and the campaign they belonged to -- so this is durable storage,
// not a query against a metrics system that will have forgotten.
type ReportStore interface {
	// SaveReport stores a run's report. Saving the same run twice replaces the
	// earlier report rather than adding a second: a run has one outcome, and a
	// retry after a partial failure must not leave two disagreeing records.
	SaveReport(ctx context.Context, r report.Report) error
	// GetReport returns a run's report, or ErrNotFound.
	GetReport(ctx context.Context, runID int64) (report.Report, error)
	// ListReports returns an execution's reports, most recent first, so a
	// service owner can see how it has behaved over time.
	//
	// A limit of zero or less means no limit. Note that this is the opposite of
	// SQL's LIMIT 0, which returns nothing -- an implementation must omit the
	// clause rather than pass the value through.
	ListReports(ctx context.Context, executionID int64, limit int) ([]report.Report, error)
	// ReportsSince returns reports across all executions started at or after
	// the given time, which is what trend analytics reads. The boundary is
	// inclusive; limit behaves as it does for ListReports.
	ReportsSince(ctx context.Context, since time.Time, limit int) ([]report.Report, error)
}
