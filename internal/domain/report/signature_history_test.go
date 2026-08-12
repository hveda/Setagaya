package report_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/domain/report"
)

func sigRow(label, code string, side report.Side, total int64, runs int) report.SignatureHistoryRow {
	return report.SignatureHistoryRow{
		Signature:  report.Signature{Label: label, ResponseCode: code, Side: side},
		TotalCount: total, RunCount: runs,
	}
}

func TestGroupSignatureHistory_ByLabel_SumsAcrossResponseCodes(t *testing.T) {
	t.Parallel()
	rows := []report.SignatureHistoryRow{
		sigRow("checkout", "500", report.SideTarget, 7, 2),
		sigRow("checkout", "404", report.SideTarget, 1, 1),
		sigRow("cart", "500", report.SideTarget, 3, 1),
	}
	got := report.GroupSignatureHistory(rows, report.GroupByLabel)
	if len(got) != 2 {
		t.Fatalf("GroupSignatureHistory = %+v, want 2 groups", got)
	}
	// Dominant first: "checkout" totals 8, "cart" totals 3.
	if got[0].Key != "checkout" || got[0].TotalCount != 8 || len(got[0].Rows) != 2 {
		t.Fatalf("group[0] = %+v, want checkout totalled 8 across 2 rows", got[0])
	}
	if got[1].Key != "cart" || got[1].TotalCount != 3 || len(got[1].Rows) != 1 {
		t.Fatalf("group[1] = %+v, want cart totalled 3 across 1 row", got[1])
	}
}

func TestGroupSignatureHistory_ByResponseCode_SumsAcrossLabels(t *testing.T) {
	t.Parallel()
	rows := []report.SignatureHistoryRow{
		sigRow("checkout", "500", report.SideTarget, 7, 2),
		sigRow("cart", "500", report.SideTarget, 3, 1),
		sigRow("checkout", "404", report.SideTarget, 1, 1),
	}
	got := report.GroupSignatureHistory(rows, report.GroupByResponseCode)
	if len(got) != 2 {
		t.Fatalf("GroupSignatureHistory = %+v, want 2 groups", got)
	}
	if got[0].Key != "500" || got[0].TotalCount != 10 || len(got[0].Rows) != 2 {
		t.Fatalf("group[0] = %+v, want 500 totalled 10 across 2 rows", got[0])
	}
	if got[1].Key != "404" || got[1].TotalCount != 1 {
		t.Fatalf("group[1] = %+v, want 404 totalled 1", got[1])
	}
}

// RunCount is deliberately not rolled up at the group level -- a reader
// wanting run coverage reads the leaf Rows, since summing RunCount across
// rows could double count a run that produced two response codes under one
// label.
func TestGroupSignatureHistory_DoesNotRollUpRunCount(t *testing.T) {
	t.Parallel()
	rows := []report.SignatureHistoryRow{
		sigRow("checkout", "500", report.SideTarget, 5, 3),
		sigRow("checkout", "404", report.SideTarget, 2, 3), // same 3 runs, different code
	}
	got := report.GroupSignatureHistory(rows, report.GroupByLabel)
	if len(got) != 1 {
		t.Fatalf("GroupSignatureHistory = %+v, want 1 group", got)
	}
	if got[0].TotalCount != 7 {
		t.Fatalf("group[0].TotalCount = %d, want 7 (5+2, a safe re-sum)", got[0].TotalCount)
	}
	// Explicitly: there is no group-level RunCount field to check -- the
	// leaf rows (each carrying its own RunCount) are the source of truth.
	if len(got[0].Rows) != 2 || got[0].Rows[0].RunCount != 3 || got[0].Rows[1].RunCount != 3 {
		t.Fatalf("group[0].Rows = %+v, want both leaf rows' own RunCount preserved", got[0].Rows)
	}
}

func TestGroupSignatureHistory_Empty(t *testing.T) {
	t.Parallel()
	got := report.GroupSignatureHistory(nil, report.GroupByLabel)
	if len(got) != 0 {
		t.Fatalf("GroupSignatureHistory(nil) = %+v, want empty", got)
	}
}

func TestGroupSignatureHistory_TieBrokenByKey(t *testing.T) {
	t.Parallel()
	rows := []report.SignatureHistoryRow{
		sigRow("zebra", "500", report.SideTarget, 5, 1),
		sigRow("apple", "500", report.SideTarget, 5, 1),
	}
	got := report.GroupSignatureHistory(rows, report.GroupByLabel)
	if len(got) != 2 || got[0].Key != "apple" || got[1].Key != "zebra" {
		t.Fatalf("GroupSignatureHistory = %+v, want apple before zebra on a tied count", got)
	}
}
