package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMakeName(t *testing.T) {
	testCases := []struct {
		name         string
		kind         string
		projectID    int64
		collectionID int64
		planID       int64
		engineID     int
		expected     string
	}{
		{
			name:         "engine name generation",
			kind:         "engine",
			projectID:    123,
			collectionID: 456,
			planID:       789,
			engineID:     1,
			expected:     "engine-123-456-789-1",
		},
		{
			name:         "ingress name generation",
			kind:         "ingress",
			projectID:    100,
			collectionID: 200,
			planID:       300,
			engineID:     5,
			expected:     "ingress-100-200-300-5",
		},
		{
			name:         "zero values",
			kind:         "test",
			projectID:    0,
			collectionID: 0,
			planID:       0,
			engineID:     0,
			expected:     "test-0-0-0-0",
		},
		{
			name:         "negative values",
			kind:         "test",
			projectID:    -1,
			collectionID: -2,
			planID:       -3,
			engineID:     -4,
			expected:     "test--1--2--3--4",
		},
		{
			name:         "large values",
			kind:         "large",
			projectID:    9223372036854775807, // max int64
			collectionID: 9223372036854775806,
			planID:       9223372036854775805,
			engineID:     2147483647, // max int32
			expected:     "large-9223372036854775807-9223372036854775806-9223372036854775805-2147483647",
		},
		{
			name:         "empty kind",
			kind:         "",
			projectID:    1,
			collectionID: 2,
			planID:       3,
			engineID:     4,
			expected:     "-1-2-3-4",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeName(tc.kind, tc.projectID, tc.collectionID, tc.planID, tc.engineID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMakeEngineName(t *testing.T) {
	testCases := []struct {
		name         string
		projectID    int64
		collectionID int64
		planID       int64
		engineID     int
		expected     string
	}{
		{
			name:         "typical engine name",
			projectID:    1,
			collectionID: 2,
			planID:       3,
			engineID:     4,
			expected:     "engine-1-2-3-4",
		},
		{
			name:         "single digit IDs",
			projectID:    5,
			collectionID: 6,
			planID:       7,
			engineID:     8,
			expected:     "engine-5-6-7-8",
		},
		{
			name:         "multi digit IDs",
			projectID:    123,
			collectionID: 456,
			planID:       789,
			engineID:     101,
			expected:     "engine-123-456-789-101",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeEngineName(tc.projectID, tc.collectionID, tc.planID, tc.engineID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMakePlanName(t *testing.T) {
	testCases := []struct {
		name         string
		projectID    int64
		collectionID int64
		planID       int64
		expected     string
	}{
		{
			name:         "typical plan name",
			projectID:    1,
			collectionID: 2,
			planID:       3,
			expected:     "engine-1-2-3",
		},
		{
			name:         "large IDs",
			projectID:    999999,
			collectionID: 888888,
			planID:       777777,
			expected:     "engine-999999-888888-777777",
		},
		{
			name:         "zero IDs",
			projectID:    0,
			collectionID: 0,
			planID:       0,
			expected:     "engine-0-0-0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makePlanName(tc.projectID, tc.collectionID, tc.planID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMakeIngressName(t *testing.T) {
	testCases := []struct {
		name         string
		projectID    int64
		collectionID int64
		planID       int64
		engineID     int
		expected     string
	}{
		{
			name:         "typical ingress name",
			projectID:    10,
			collectionID: 20,
			planID:       30,
			engineID:     40,
			expected:     "ingress-10-20-30-40",
		},
		{
			name:         "single values",
			projectID:    1,
			collectionID: 1,
			planID:       1,
			engineID:     1,
			expected:     "ingress-1-1-1-1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeIngressName(tc.projectID, tc.collectionID, tc.planID, tc.engineID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMakeIngressClass(t *testing.T) {
	testCases := []struct {
		name      string
		projectID int64
		expected  string
	}{
		{
			name:      "typical project ID",
			projectID: 123,
			expected:  "ig-123",
		},
		{
			name:      "single digit",
			projectID: 5,
			expected:  "ig-5",
		},
		{
			name:      "zero project ID",
			projectID: 0,
			expected:  "ig-0",
		},
		{
			name:      "large project ID",
			projectID: 9999999999,
			expected:  "ig-9999999999",
		},
		{
			name:      "negative project ID",
			projectID: -123,
			expected:  "ig--123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeIngressClass(tc.projectID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestMakeBaseLabel(t *testing.T) {
	testCases := []struct {
		name         string
		projectID    int64
		collectionID int64
		expected     map[string]string
	}{
		{
			name:         "typical IDs",
			projectID:    123,
			collectionID: 456,
			expected: map[string]string{
				"collection": "456",
				"project":    "123",
			},
		},
		{
			name:         "zero IDs",
			projectID:    0,
			collectionID: 0,
			expected: map[string]string{
				"collection": "0",
				"project":    "0",
			},
		},
		{
			name:         "large IDs",
			projectID:    9223372036854775807,
			collectionID: 9223372036854775806,
			expected: map[string]string{
				"collection": "9223372036854775806",
				"project":    "9223372036854775807",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeBaseLabel(tc.projectID, tc.collectionID)
			assert.Equal(t, tc.expected, result)
			assert.Len(t, result, 2)
			assert.Contains(t, result, "collection")
			assert.Contains(t, result, "project")
		})
	}
}

func TestMakeIngressControllerLabel(t *testing.T) {
	testCases := []struct {
		name      string
		projectID int64
		expected  map[string]string
	}{
		{
			name:      "typical project ID",
			projectID: 123,
			expected: map[string]string{
				"kind":    "ingress-controller",
				"project": "123",
			},
		},
		{
			name:      "zero project ID",
			projectID: 0,
			expected: map[string]string{
				"kind":    "ingress-controller",
				"project": "0",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeIngressControllerLabel(tc.projectID)
			assert.Equal(t, tc.expected, result)
			assert.Len(t, result, 2)
			assert.Equal(t, "ingress-controller", result["kind"])
		})
	}
}

func TestMakeIngressLabel(t *testing.T) {
	testCases := []struct {
		name         string
		projectID    int64
		collectionID int64
	}{
		{
			name:         "typical IDs",
			projectID:    100,
			collectionID: 200,
		},
		{
			name:         "same IDs",
			projectID:    123,
			collectionID: 123,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeIngressLabel(tc.projectID, tc.collectionID)
			baseResult := makeBaseLabel(tc.projectID, tc.collectionID)
			
			// makeIngressLabel should return the same as makeBaseLabel
			assert.Equal(t, baseResult, result)
		})
	}
}

func TestMakeEngineLabel(t *testing.T) {
	testCases := []struct {
		name         string
		projectID    int64
		collectionID int64
		planID       int64
		engineName   string
		expected     map[string]string
	}{
		{
			name:         "typical engine label",
			projectID:    10,
			collectionID: 20,
			planID:       30,
			engineName:   "test-engine",
			expected: map[string]string{
				"collection": "20",
				"project":    "10",
				"app":        "test-engine",
				"plan":       "30",
				"kind":       "executor",
			},
		},
		{
			name:         "empty engine name",
			projectID:    1,
			collectionID: 2,
			planID:       3,
			engineName:   "",
			expected: map[string]string{
				"collection": "2",
				"project":    "1",
				"app":        "",
				"plan":       "3",
				"kind":       "executor",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeEngineLabel(tc.projectID, tc.collectionID, tc.planID, tc.engineName)
			assert.Equal(t, tc.expected, result)
			assert.Len(t, result, 5)
			assert.Equal(t, "executor", result["kind"])
		})
	}
}

func TestMakePlanLabel(t *testing.T) {
	testCases := []struct {
		name         string
		projectID    int64
		collectionID int64
		planID       int64
		expected     map[string]string
	}{
		{
			name:         "typical plan label",
			projectID:    100,
			collectionID: 200,
			planID:       300,
			expected: map[string]string{
				"collection": "200",
				"project":    "100",
				"plan":       "300",
				"kind":       "executor",
			},
		},
		{
			name:         "zero values",
			projectID:    0,
			collectionID: 0,
			planID:       0,
			expected: map[string]string{
				"collection": "0",
				"project":    "0",
				"plan":       "0",
				"kind":       "executor",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makePlanLabel(tc.projectID, tc.collectionID, tc.planID)
			assert.Equal(t, tc.expected, result)
			assert.Len(t, result, 4)
			assert.Equal(t, "executor", result["kind"])
		})
	}
}

func TestMakeCollectionLabel(t *testing.T) {
	testCases := []struct {
		name         string
		collectionID int64
		expected     string
	}{
		{
			name:         "typical collection ID",
			collectionID: 123,
			expected:     "collection=123",
		},
		{
			name:         "zero collection ID",
			collectionID: 0,
			expected:     "collection=0",
		},
		{
			name:         "large collection ID",
			collectionID: 9999999999,
			expected:     "collection=9999999999",
		},
		{
			name:         "negative collection ID",
			collectionID: -456,
			expected:     "collection=-456",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeCollectionLabel(tc.collectionID)
			assert.Equal(t, tc.expected, result)
			assert.Contains(t, result, "collection=")
		})
	}
}

func TestLabelConsistency(t *testing.T) {
	// Test that label generation is consistent
	projectID := int64(123)
	collectionID := int64(456)
	planID := int64(789)
	
	baseLabel := makeBaseLabel(projectID, collectionID)
	engineLabel := makeEngineLabel(projectID, collectionID, planID, "test-engine")
	planLabel := makePlanLabel(projectID, collectionID, planID)
	
	// All labels should have consistent base fields
	assert.Equal(t, baseLabel["collection"], engineLabel["collection"])
	assert.Equal(t, baseLabel["project"], engineLabel["project"])
	assert.Equal(t, baseLabel["collection"], planLabel["collection"])
	assert.Equal(t, baseLabel["project"], planLabel["project"])
	
	// Engine and plan labels should have consistent plan field
	assert.Equal(t, engineLabel["plan"], planLabel["plan"])
	
	// Engine and plan labels should both be "executor" kind
	assert.Equal(t, "executor", engineLabel["kind"])
	assert.Equal(t, "executor", planLabel["kind"])
}