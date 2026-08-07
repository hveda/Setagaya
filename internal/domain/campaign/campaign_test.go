package campaign_test

import (
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/campaign"
)

func at(seconds int) time.Time {
	return time.Unix(int64(seconds), 0).UTC()
}

func validCampaign() campaign.Campaign {
	return campaign.Campaign{
		Name:     "Supersale 11.11",
		TenantID: 1,
		Window:   campaign.Window{Start: at(0), End: at(100)},
		Services: []campaign.Service{
			{ProjectID: 1, ExecutionID: 10},
			{ProjectID: 2, ExecutionID: 20},
		},
	}
}

func TestCampaign_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mut  func(c campaign.Campaign) campaign.Campaign
		want error
	}{
		{"valid", func(c campaign.Campaign) campaign.Campaign { return c }, nil},
		{"empty name", func(c campaign.Campaign) campaign.Campaign { c.Name = ""; return c }, campaign.ErrNameRequired},
		{"window end equals start", func(c campaign.Campaign) campaign.Campaign {
			c.Window = campaign.Window{Start: at(10), End: at(10)}
			return c
		}, campaign.ErrWindowInvalid},
		{"window end before start", func(c campaign.Campaign) campaign.Campaign {
			c.Window = campaign.Window{Start: at(10), End: at(0)}
			return c
		}, campaign.ErrWindowInvalid},
		{"no services", func(c campaign.Campaign) campaign.Campaign { c.Services = nil; return c }, campaign.ErrServicesRequired},
		{"zero project id", func(c campaign.Campaign) campaign.Campaign {
			c.Services = []campaign.Service{{ProjectID: 0, ExecutionID: 10}}
			return c
		}, campaign.ErrProjectRequired},
		{"zero execution id", func(c campaign.Campaign) campaign.Campaign {
			c.Services = []campaign.Service{{ProjectID: 1, ExecutionID: 0}}
			return c
		}, campaign.ErrServiceExecutionInvalid},
		{"duplicate project", func(c campaign.Campaign) campaign.Campaign {
			c.Services = []campaign.Service{{ProjectID: 1, ExecutionID: 10}, {ProjectID: 1, ExecutionID: 11}}
			return c
		}, campaign.ErrDuplicateService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.mut(validCampaign()).Validate()
			if tt.want == nil {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

// IsActive is half-open on Start (inclusive) and End (exclusive), the same
// convention reservation.Overlaps and schedule.Occurrences already use, so
// a campaign that ends exactly when another begins has no ambiguous instant.
func TestCampaign_IsActive(t *testing.T) {
	t.Parallel()
	c := validCampaign() // window [0, 100)

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"before window", at(-1), false},
		{"at window start", at(0), true},
		{"inside window", at(50), true},
		{"one instant before window end", at(99), true},
		{"at window end", at(100), false},
		{"after window end", at(200), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := c.IsActive(tt.now); got != tt.want {
				t.Errorf("IsActive(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestCampaign_IsActive_AbortedIsNeverActive(t *testing.T) {
	t.Parallel()
	c := validCampaign() // window [0, 100)
	abortedAt := at(50)
	c.AbortedAt = &abortedAt

	if c.IsActive(at(50)) {
		t.Error("an aborted campaign must not be active, even inside its own window")
	}
}

func TestCampaign_DesignatedExecution(t *testing.T) {
	t.Parallel()
	c := validCampaign()

	if got, ok := c.DesignatedExecution(1); !ok || got != 10 {
		t.Errorf("DesignatedExecution(1) = %d, %v, want 10, true", got, ok)
	}
	if got, ok := c.DesignatedExecution(2); !ok || got != 20 {
		t.Errorf("DesignatedExecution(2) = %d, %v, want 20, true", got, ok)
	}
	if _, ok := c.DesignatedExecution(999); ok {
		t.Error("DesignatedExecution(999) = ok, want not-found for a non-participating project")
	}
}
