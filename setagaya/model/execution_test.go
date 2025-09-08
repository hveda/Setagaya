package model

import (
	"encoding/json"
	"testing"

	yaml "gopkg.in/yaml.v2"

	"github.com/stretchr/testify/assert"
)

func TestExecutionPlan(t *testing.T) {
	// Test ExecutionPlan struct creation and field access
	ep := &ExecutionPlan{
		Name:        "test-plan",
		PlanID:      123,
		Concurrency: 10,
		Rampup:      5,
		Engines:     2,
		Duration:    300,
		CSVSplit:    true,
	}

	assert.Equal(t, "test-plan", ep.Name)
	assert.Equal(t, int64(123), ep.PlanID)
	assert.Equal(t, 10, ep.Concurrency)
	assert.Equal(t, 5, ep.Rampup)
	assert.Equal(t, 2, ep.Engines)
	assert.Equal(t, 300, ep.Duration)
	assert.True(t, ep.CSVSplit)
}

func TestExecutionPlanJSONSerialization(t *testing.T) {
	ep := &ExecutionPlan{
		Name:        "test-plan",
		PlanID:      123,
		Concurrency: 10,
		Rampup:      5,
		Engines:     2,
		Duration:    300,
		CSVSplit:    true,
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(ep)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), `"name":"test-plan"`)
	assert.Contains(t, string(jsonData), `"plan_id":123`)
	assert.Contains(t, string(jsonData), `"concurrency":10`)
	assert.Contains(t, string(jsonData), `"csv_split":true`)

	// Test JSON unmarshaling
	var unmarshaledEP ExecutionPlan
	err = json.Unmarshal(jsonData, &unmarshaledEP)
	assert.NoError(t, err)
	assert.Equal(t, ep.Name, unmarshaledEP.Name)
	assert.Equal(t, ep.PlanID, unmarshaledEP.PlanID)
	assert.Equal(t, ep.Concurrency, unmarshaledEP.Concurrency)
	assert.Equal(t, ep.CSVSplit, unmarshaledEP.CSVSplit)
}

func TestExecutionPlanYAMLSerialization(t *testing.T) {
	ep := &ExecutionPlan{
		Name:        "test-plan",
		PlanID:      123,
		Concurrency: 10,
		Rampup:      5,
		Engines:     2,
		Duration:    300,
		CSVSplit:    false,
	}

	// Test YAML marshaling
	yamlData, err := yaml.Marshal(ep)
	assert.NoError(t, err)
	assert.Contains(t, string(yamlData), "name: test-plan")
	assert.Contains(t, string(yamlData), "testid: 123")
	assert.Contains(t, string(yamlData), "concurrency: 10")
	assert.Contains(t, string(yamlData), "csv_split: false")

	// Test YAML unmarshaling
	var unmarshaledEP ExecutionPlan
	err = yaml.Unmarshal(yamlData, &unmarshaledEP)
	assert.NoError(t, err)
	assert.Equal(t, ep.Name, unmarshaledEP.Name)
	assert.Equal(t, ep.PlanID, unmarshaledEP.PlanID)
	assert.Equal(t, ep.Concurrency, unmarshaledEP.Concurrency)
	assert.Equal(t, ep.CSVSplit, unmarshaledEP.CSVSplit)
}

func TestExecutionCollection(t *testing.T) {
	// Create test execution plans
	ep1 := &ExecutionPlan{
		Name:        "plan-1",
		PlanID:      1,
		Concurrency: 5,
		Rampup:      2,
		Engines:     1,
		Duration:    60,
		CSVSplit:    false,
	}

	ep2 := &ExecutionPlan{
		Name:        "plan-2",
		PlanID:      2,
		Concurrency: 10,
		Rampup:      3,
		Engines:     2,
		Duration:    120,
		CSVSplit:    true,
	}

	// Test ExecutionCollection struct
	ec := &ExecutionCollection{
		Name:         "test-collection",
		ProjectID:    456,
		CollectionID: 789,
		Tests:        []*ExecutionPlan{ep1, ep2},
		CSVSplit:     true,
	}

	assert.Equal(t, "test-collection", ec.Name)
	assert.Equal(t, int64(456), ec.ProjectID)
	assert.Equal(t, int64(789), ec.CollectionID)
	assert.Equal(t, 2, len(ec.Tests))
	assert.True(t, ec.CSVSplit)

	// Verify nested execution plans
	assert.Equal(t, "plan-1", ec.Tests[0].Name)
	assert.Equal(t, "plan-2", ec.Tests[1].Name)
	assert.Equal(t, int64(1), ec.Tests[0].PlanID)
	assert.Equal(t, int64(2), ec.Tests[1].PlanID)
}

func TestExecutionCollectionYAMLSerialization(t *testing.T) {
	ep := &ExecutionPlan{
		Name:        "test-plan",
		PlanID:      123,
		Concurrency: 10,
		Rampup:      5,
		Engines:     2,
		Duration:    300,
		CSVSplit:    true,
	}

	ec := &ExecutionCollection{
		Name:         "test-collection",
		ProjectID:    456,
		CollectionID: 789,
		Tests:        []*ExecutionPlan{ep},
		CSVSplit:     false,
	}

	// Test YAML marshaling
	yamlData, err := yaml.Marshal(ec)
	assert.NoError(t, err)
	assert.Contains(t, string(yamlData), "name: test-collection")
	assert.Contains(t, string(yamlData), "projectid: 456")
	assert.Contains(t, string(yamlData), "collectionid: 789")
	assert.Contains(t, string(yamlData), "csv_split: false")

	// Test YAML unmarshaling
	var unmarshaledEC ExecutionCollection
	err = yaml.Unmarshal(yamlData, &unmarshaledEC)
	assert.NoError(t, err)
	assert.Equal(t, ec.Name, unmarshaledEC.Name)
	assert.Equal(t, ec.ProjectID, unmarshaledEC.ProjectID)
	assert.Equal(t, ec.CollectionID, unmarshaledEC.CollectionID)
	assert.Equal(t, len(ec.Tests), len(unmarshaledEC.Tests))
	assert.Equal(t, ec.CSVSplit, unmarshaledEC.CSVSplit)

	// Verify nested execution plan
	if len(unmarshaledEC.Tests) > 0 {
		assert.Equal(t, ep.Name, unmarshaledEC.Tests[0].Name)
		assert.Equal(t, ep.PlanID, unmarshaledEC.Tests[0].PlanID)
	}
}

func TestExecutionWrapper(t *testing.T) {
	ep := &ExecutionPlan{
		Name:        "wrapped-plan",
		PlanID:      999,
		Concurrency: 20,
		Rampup:      10,
		Engines:     5,
		Duration:    600,
		CSVSplit:    true,
	}

	ec := &ExecutionCollection{
		Name:         "wrapped-collection",
		ProjectID:    111,
		CollectionID: 222,
		Tests:        []*ExecutionPlan{ep},
		CSVSplit:     true,
	}

	// Test ExecutionWrapper struct
	ew := &ExecutionWrapper{
		Content: ec,
	}

	assert.NotNil(t, ew.Content)
	assert.Equal(t, "wrapped-collection", ew.Content.Name)
	assert.Equal(t, 1, len(ew.Content.Tests))
	assert.Equal(t, "wrapped-plan", ew.Content.Tests[0].Name)
}

func TestExecutionWrapperYAMLSerialization(t *testing.T) {
	ep := &ExecutionPlan{
		Name:        "wrapped-plan",
		PlanID:      999,
		Concurrency: 20,
		Rampup:      10,
		Engines:     5,
		Duration:    600,
		CSVSplit:    false,
	}

	ec := &ExecutionCollection{
		Name:         "wrapped-collection",
		ProjectID:    111,
		CollectionID: 222,
		Tests:        []*ExecutionPlan{ep},
		CSVSplit:     true,
	}

	ew := &ExecutionWrapper{
		Content: ec,
	}

	// Test YAML marshaling
	yamlData, err := yaml.Marshal(ew)
	assert.NoError(t, err)
	assert.Contains(t, string(yamlData), "multi-test:")
	assert.Contains(t, string(yamlData), "name: wrapped-collection")

	// Test YAML unmarshaling
	var unmarshaledEW ExecutionWrapper
	err = yaml.Unmarshal(yamlData, &unmarshaledEW)
	assert.NoError(t, err)
	assert.NotNil(t, unmarshaledEW.Content)
	assert.Equal(t, ew.Content.Name, unmarshaledEW.Content.Name)
	assert.Equal(t, ew.Content.ProjectID, unmarshaledEW.Content.ProjectID)
	assert.Equal(t, ew.Content.CollectionID, unmarshaledEW.Content.CollectionID)
	assert.Equal(t, len(ew.Content.Tests), len(unmarshaledEW.Content.Tests))
}

func TestExecutionStructsEdgeCases(t *testing.T) {
	t.Run("empty execution plan", func(t *testing.T) {
		ep := &ExecutionPlan{}
		assert.Equal(t, "", ep.Name)
		assert.Equal(t, int64(0), ep.PlanID)
		assert.Equal(t, 0, ep.Concurrency)
		assert.False(t, ep.CSVSplit)
	})

	t.Run("empty execution collection", func(t *testing.T) {
		ec := &ExecutionCollection{}
		assert.Equal(t, "", ec.Name)
		assert.Equal(t, int64(0), ec.ProjectID)
		assert.Nil(t, ec.Tests)
		assert.False(t, ec.CSVSplit)
	})

	t.Run("execution collection with no tests", func(t *testing.T) {
		ec := &ExecutionCollection{
			Name:         "empty-collection",
			ProjectID:    123,
			CollectionID: 456,
			Tests:        []*ExecutionPlan{},
			CSVSplit:     false,
		}
		assert.Equal(t, 0, len(ec.Tests))
		assert.Equal(t, "empty-collection", ec.Name)
		assert.Equal(t, int64(123), ec.ProjectID)
		assert.Equal(t, int64(456), ec.CollectionID)
		assert.False(t, ec.CSVSplit)
	})

	t.Run("negative values in execution plan", func(t *testing.T) {
		ep := &ExecutionPlan{
			Name:        "negative-test",
			PlanID:      -1,
			Concurrency: -5,
			Rampup:      -2,
			Engines:     -1,
			Duration:    -300,
			CSVSplit:    false,
		}
		assert.Equal(t, "negative-test", ep.Name)
		assert.Equal(t, int64(-1), ep.PlanID)
		assert.Equal(t, -5, ep.Concurrency)
		assert.Equal(t, -2, ep.Rampup)
		assert.Equal(t, -1, ep.Engines)
		assert.Equal(t, -300, ep.Duration)
		assert.False(t, ep.CSVSplit)
	})

	t.Run("very large values", func(t *testing.T) {
		ep := &ExecutionPlan{
			Name:        "large-test",
			PlanID:      9223372036854775807, // max int64
			Concurrency: 2147483647,          // max int32
			Rampup:      2147483647,
			Engines:     2147483647,
			Duration:    2147483647,
			CSVSplit:    true,
		}
		assert.Equal(t, "large-test", ep.Name)
		assert.Equal(t, int64(9223372036854775807), ep.PlanID)
		assert.Equal(t, 2147483647, ep.Concurrency)
		assert.Equal(t, 2147483647, ep.Rampup)
		assert.Equal(t, 2147483647, ep.Engines)
		assert.Equal(t, 2147483647, ep.Duration)
		assert.True(t, ep.CSVSplit)
	})
}

// Test real-world YAML configuration
func TestRealWorldYAMLConfiguration(t *testing.T) {
	yamlConfig := `
multi-test:
  name: "Load Test Collection"
  projectid: 100
  collectionid: 200
  csv_split: true
  tests:
    - name: "API Load Test"
      testid: 1
      concurrency: 50
      rampup: 10
      engines: 3
      duration: 1800
      csv_split: false
    - name: "Database Load Test"
      testid: 2
      concurrency: 20
      rampup: 5
      engines: 2
      duration: 900
      csv_split: true
`

	var wrapper ExecutionWrapper
	err := yaml.Unmarshal([]byte(yamlConfig), &wrapper)
	assert.NoError(t, err)

	assert.NotNil(t, wrapper.Content)
	assert.Equal(t, "Load Test Collection", wrapper.Content.Name)
	assert.Equal(t, int64(100), wrapper.Content.ProjectID)
	assert.Equal(t, int64(200), wrapper.Content.CollectionID)
	assert.True(t, wrapper.Content.CSVSplit)
	assert.Equal(t, 2, len(wrapper.Content.Tests))

	// Check first test
	test1 := wrapper.Content.Tests[0]
	assert.Equal(t, "API Load Test", test1.Name)
	assert.Equal(t, int64(1), test1.PlanID)
	assert.Equal(t, 50, test1.Concurrency)
	assert.Equal(t, 10, test1.Rampup)
	assert.Equal(t, 3, test1.Engines)
	assert.Equal(t, 1800, test1.Duration)
	assert.False(t, test1.CSVSplit)

	// Check second test
	test2 := wrapper.Content.Tests[1]
	assert.Equal(t, "Database Load Test", test2.Name)
	assert.Equal(t, int64(2), test2.PlanID)
	assert.Equal(t, 20, test2.Concurrency)
	assert.Equal(t, 5, test2.Rampup)
	assert.Equal(t, 2, test2.Engines)
	assert.Equal(t, 900, test2.Duration)
	assert.True(t, test2.CSVSplit)
}
