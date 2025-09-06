package model

import (
	"testing"
	"time"
	"math"

	"github.com/stretchr/testify/assert"
)

func TestCalBillingHours(t *testing.T) {
	testCases := []struct {
		name        string
		startTime   time.Time
		endTime     time.Time
		expectedHours float64
	}{
		{
			name:        "exactly one hour",
			startTime:   time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2023, 1, 1, 11, 0, 0, 0, time.UTC),
			expectedHours: 1,
		},
		{
			name:        "less than one hour - should round up",
			startTime:   time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2023, 1, 1, 10, 30, 0, 0, time.UTC),
			expectedHours: 1,
		},
		{
			name:        "slightly over one hour - should round up",
			startTime:   time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2023, 1, 1, 11, 0, 1, 0, time.UTC),
			expectedHours: 2,
		},
		{
			name:        "two and half hours - should round up",
			startTime:   time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC),
			expectedHours: 3,
		},
		{
			name:        "exactly zero duration",
			startTime:   time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			expectedHours: 0,
		},
		{
			name:        "very small duration - should round up to 1",
			startTime:   time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2023, 1, 1, 10, 0, 1, 0, time.UTC),
			expectedHours: 1,
		},
		{
			name:        "multiple days",
			startTime:   time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC),
			endTime:     time.Date(2023, 1, 3, 10, 0, 0, 0, time.UTC),
			expectedHours: 48,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calBillingHours(tc.startTime, tc.endTime)
			assert.Equal(t, tc.expectedHours, result)
		})
	}
}

func TestCalVUH(t *testing.T) {
	testCases := []struct {
		name         string
		billingHours float64
		vu           float64
		expectedVUH  float64
	}{
		{
			name:         "simple calculation",
			billingHours: 2,
			vu:           10,
			expectedVUH:  20,
		},
		{
			name:         "zero hours",
			billingHours: 0,
			vu:           10,
			expectedVUH:  0,
		},
		{
			name:         "zero virtual users",
			billingHours: 2,
			vu:           0,
			expectedVUH:  0,
		},
		{
			name:         "fractional hours",
			billingHours: 1.5,
			vu:           4,
			expectedVUH:  6,
		},
		{
			name:         "large numbers",
			billingHours: 24,
			vu:           1000,
			expectedVUH:  24000,
		},
		{
			name:         "decimal virtual users",
			billingHours: 3,
			vu:           2.5,
			expectedVUH:  7.5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calVUH(tc.billingHours, tc.vu)
			assert.Equal(t, tc.expectedVUH, result)
		})
	}
}

func TestFindOwner(t *testing.T) {
	testProject := Project{
		ID:    1,
		Name:  "test-project",
		Owner: "project-owner",
		SID:   "project-sid",
	}

	testCases := []struct {
		name         string
		owner        string
		project      Project
		expectedOwner string
	}{
		{
			name:         "numeric owner - return SID",
			owner:        "12345",
			project:      testProject,
			expectedOwner: "project-sid",
		},
		{
			name:         "non-numeric owner - return owner",
			owner:        "user@example.com",
			project:      testProject,
			expectedOwner: "user@example.com",
		},
		{
			name:         "alphabetic owner - return owner",
			owner:        "username",
			project:      testProject,
			expectedOwner: "username",
		},
		{
			name:         "mixed alphanumeric owner - return owner",
			owner:        "user123",
			project:      testProject,
			expectedOwner: "user123",
		},
		{
			name:         "empty owner with SID",
			owner:        "",
			project:      testProject,
			expectedOwner: "",
		},
		{
			name: "numeric owner with empty SID",
			owner: "12345",
			project: Project{
				ID:    1,
				Name:  "test-project",
				Owner: "project-owner",
				SID:   "",
			},
			expectedOwner: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := findOwner(tc.owner, tc.project)
			assert.Equal(t, tc.expectedOwner, result)
		})
	}
}

func TestUnitUsageStruct(t *testing.T) {
	// Test UnitUsage struct creation and manipulation
	uu := UnitUsage{
		TotalVUH: make(map[string]float64),
	}

	assert.NotNil(t, uu.TotalVUH)
	assert.Equal(t, 0, len(uu.TotalVUH))

	// Test adding values
	uu.TotalVUH["context1"] = 100.5
	uu.TotalVUH["context2"] = 200.0

	assert.Equal(t, 100.5, uu.TotalVUH["context1"])
	assert.Equal(t, 200.0, uu.TotalVUH["context2"])
	assert.Equal(t, 2, len(uu.TotalVUH))
}

func TestTotalUsageSummaryStruct(t *testing.T) {
	// Test TotalUsageSummary struct creation
	tus := &TotalUsageSummary{
		UnitUsage: UnitUsage{
			TotalVUH: make(map[string]float64),
		},
		VUHByOnwer: make(map[string]map[string]float64),
		Contacts:   make(map[string][]string),
	}

	assert.NotNil(t, tus.TotalVUH)
	assert.NotNil(t, tus.VUHByOnwer)
	assert.NotNil(t, tus.Contacts)

	// Test nested map structure
	tus.VUHByOnwer["owner1"] = make(map[string]float64)
	tus.VUHByOnwer["owner1"]["context1"] = 50.0

	assert.Equal(t, 50.0, tus.VUHByOnwer["owner1"]["context1"])

	// Test contacts array
	tus.Contacts["sid1"] = []string{"contact1@example.com", "contact2@example.com"}
	assert.Equal(t, 2, len(tus.Contacts["sid1"]))
	assert.Contains(t, tus.Contacts["sid1"], "contact1@example.com")
}

func TestOwnerUsageSummaryStruct(t *testing.T) {
	// Test OwnerUsageSummary struct creation
	ous := &OwnerUsageSummary{
		UnitUsage: UnitUsage{
			TotalVUH: make(map[string]float64),
		},
		History: []*CollectionLaunchHistory{},
	}

	assert.NotNil(t, ous.TotalVUH)
	assert.NotNil(t, ous.History)
	assert.Equal(t, 0, len(ous.History))

	// Test adding history
	history := &CollectionLaunchHistory{
		Context:      "test-context",
		CollectionID: 1,
		Owner:        "test-owner",
		Vu:           10,
		StartedTime:  time.Now(),
		EndTime:      time.Now().Add(time.Hour),
		BillingHours: 1.0,
	}

	ous.History = append(ous.History, history)
	assert.Equal(t, 1, len(ous.History))
	assert.Equal(t, "test-context", ous.History[0].Context)
}

func TestCollectionLaunchHistoryStruct(t *testing.T) {
	startTime := time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2023, 1, 1, 12, 30, 0, 0, time.UTC)

	clh := &CollectionLaunchHistory{
		Context:      "production",
		CollectionID: 123,
		Owner:        "user@example.com",
		Vu:           50,
		StartedTime:  startTime,
		EndTime:      endTime,
		BillingHours: 3.0,
	}

	assert.Equal(t, "production", clh.Context)
	assert.Equal(t, int64(123), clh.CollectionID)
	assert.Equal(t, "user@example.com", clh.Owner)
	assert.Equal(t, 50, clh.Vu)
	assert.Equal(t, startTime, clh.StartedTime)
	assert.Equal(t, endTime, clh.EndTime)
	assert.Equal(t, 3.0, clh.BillingHours)
}

// Test edge cases and error conditions
func TestUsageEdgeCases(t *testing.T) {
	t.Run("negative duration billing hours", func(t *testing.T) {
		// End time before start time
		startTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		endTime := time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)
		
		result := calBillingHours(startTime, endTime)
		// Should handle negative duration (ceil of negative number)
		expected := math.Ceil(-2.0) // -2 hours
		assert.Equal(t, expected, result)
	})

	t.Run("negative VUH calculation", func(t *testing.T) {
		result := calVUH(-1.0, 10.0)
		assert.Equal(t, -10.0, result)
	})

	t.Run("very large numbers", func(t *testing.T) {
		result := calVUH(1000000, 1000000)
		assert.Equal(t, 1000000000000.0, result)
	})
}