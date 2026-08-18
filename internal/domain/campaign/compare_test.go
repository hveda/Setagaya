package campaign_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/domain/campaign"
)

func TestCompare_EveryTransition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		current    []campaign.ServiceSignal
		baseline   []campaign.ServiceSignal
		wantStatus campaign.ComparisonStatus
		wantCurr   bool
		wantGo     bool
		wantBase   bool
		wantBaseGo bool
	}{
		{
			name:       "improved: no-go baseline, go now",
			current:    []campaign.ServiceSignal{{ProjectID: 1, Go: true}},
			baseline:   []campaign.ServiceSignal{{ProjectID: 1, Go: false}},
			wantStatus: campaign.ComparisonImproved,
			wantCurr:   true, wantGo: true, wantBase: true, wantBaseGo: false,
		},
		{
			name:       "regressed: go baseline, no-go now",
			current:    []campaign.ServiceSignal{{ProjectID: 1, Go: false}},
			baseline:   []campaign.ServiceSignal{{ProjectID: 1, Go: true}},
			wantStatus: campaign.ComparisonRegressed,
			wantCurr:   true, wantGo: false, wantBase: true, wantBaseGo: true,
		},
		{
			name:       "newly-at-risk: no-go now, absent from baseline",
			current:    []campaign.ServiceSignal{{ProjectID: 1, Go: false}},
			baseline:   nil,
			wantStatus: campaign.ComparisonNewlyAtRisk,
			wantCurr:   true, wantGo: false, wantBase: false,
		},
		{
			name:       "still-at-risk: no-go in both",
			current:    []campaign.ServiceSignal{{ProjectID: 1, Go: false}},
			baseline:   []campaign.ServiceSignal{{ProjectID: 1, Go: false}},
			wantStatus: campaign.ComparisonStillAtRisk,
			wantCurr:   true, wantGo: false, wantBase: true, wantBaseGo: false,
		},
		{
			name:       "steady: go in both",
			current:    []campaign.ServiceSignal{{ProjectID: 1, Go: true}},
			baseline:   []campaign.ServiceSignal{{ProjectID: 1, Go: true}},
			wantStatus: campaign.ComparisonSteady,
			wantCurr:   true, wantGo: true, wantBase: true, wantBaseGo: true,
		},
		{
			name:       "new: go now, absent from baseline",
			current:    []campaign.ServiceSignal{{ProjectID: 1, Go: true}},
			baseline:   nil,
			wantStatus: campaign.ComparisonNew,
			wantCurr:   true, wantGo: true, wantBase: false,
		},
		{
			name:       "dropped: present in baseline only, was go",
			current:    nil,
			baseline:   []campaign.ServiceSignal{{ProjectID: 1, Go: true}},
			wantStatus: campaign.ComparisonDropped,
			wantCurr:   false, wantBase: true, wantBaseGo: true,
		},
		{
			name:       "dropped: present in baseline only, was no-go",
			current:    nil,
			baseline:   []campaign.ServiceSignal{{ProjectID: 1, Go: false}},
			wantStatus: campaign.ComparisonDropped,
			wantCurr:   false, wantBase: true, wantBaseGo: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := campaign.Compare(tc.current, tc.baseline)
			if len(got) != 1 {
				t.Fatalf("Compare() = %+v, want exactly one row", got)
			}
			row := got[0]
			if row.ProjectID != 1 || row.Status != tc.wantStatus {
				t.Fatalf("row = %+v, want project 1 status %q", row, tc.wantStatus)
			}
			if row.HasCurrent != tc.wantCurr || (tc.wantCurr && row.Go != tc.wantGo) {
				t.Fatalf("row = %+v, want HasCurrent=%v Go=%v", row, tc.wantCurr, tc.wantGo)
			}
			if row.HasBaseline != tc.wantBase || (tc.wantBase && row.BaselineGo != tc.wantBaseGo) {
				t.Fatalf("row = %+v, want HasBaseline=%v BaselineGo=%v", row, tc.wantBase, tc.wantBaseGo)
			}
		})
	}
}

func TestCompare_OrderedByProjectID(t *testing.T) {
	t.Parallel()
	current := []campaign.ServiceSignal{{ProjectID: 30, Go: true}, {ProjectID: 10, Go: true}}
	baseline := []campaign.ServiceSignal{{ProjectID: 20, Go: false}}
	got := campaign.Compare(current, baseline)
	if len(got) != 3 {
		t.Fatalf("Compare() = %+v, want 3 rows", got)
	}
	for i, wantID := range []int64{10, 20, 30} {
		if got[i].ProjectID != wantID {
			t.Fatalf("Compare()[%d].ProjectID = %d, want %d (order = %+v)", i, got[i].ProjectID, wantID, got)
		}
	}
}

func TestCompare_BothEmpty(t *testing.T) {
	t.Parallel()
	got := campaign.Compare(nil, nil)
	if len(got) != 0 {
		t.Fatalf("Compare(nil, nil) = %+v, want empty", got)
	}
}

// A project appearing in current is matched to the same project in baseline
// by ProjectID alone -- unrelated projects at different ids never collide.
func TestCompare_MultipleProjectsIndependentlyClassified(t *testing.T) {
	t.Parallel()
	current := []campaign.ServiceSignal{{ProjectID: 1, Go: true}, {ProjectID: 2, Go: false}}
	baseline := []campaign.ServiceSignal{{ProjectID: 1, Go: false}, {ProjectID: 2, Go: false}}
	got := campaign.Compare(current, baseline)
	if len(got) != 2 {
		t.Fatalf("Compare() = %+v, want 2 rows", got)
	}
	if got[0].Status != campaign.ComparisonImproved {
		t.Errorf("project 1 = %+v, want improved", got[0])
	}
	if got[1].Status != campaign.ComparisonStillAtRisk {
		t.Errorf("project 2 = %+v, want still_at_risk", got[1])
	}
}
