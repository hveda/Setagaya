package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hveda/Setagaya/setagaya/model"
	"github.com/hveda/Setagaya/setagaya/scheduler"
)

func TestHasInvalidDiff(t *testing.T) {
	testCases := []struct {
		name          string
		current       []*model.ExecutionPlan
		updated       []*model.ExecutionPlan
		expectInvalid bool
		expectedMsg   string
	}{
		{
			name: "no changes - valid",
			current: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
				{PlanID: 2, Engines: 3, Concurrency: 15},
			},
			updated: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
				{PlanID: 2, Engines: 3, Concurrency: 15},
			},
			expectInvalid: false,
			expectedMsg:   "",
		},
		{
			name: "different number of plans - invalid",
			current: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
			},
			updated: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
				{PlanID: 2, Engines: 3, Concurrency: 15},
			},
			expectInvalid: true,
			expectedMsg:   "You cannot add/remove plans while have engines deployed",
		},
		{
			name: "fewer plans in updated - invalid",
			current: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
				{PlanID: 2, Engines: 3, Concurrency: 15},
			},
			updated: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
			},
			expectInvalid: true,
			expectedMsg:   "You cannot add/remove plans while have engines deployed",
		},
		{
			name: "new plan added - invalid",
			current: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
			},
			updated: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
				{PlanID: 3, Engines: 2, Concurrency: 10}, // Different PlanID
			},
			expectInvalid: true,
			expectedMsg:   "You cannot add/remove plans while have engines deployed",
		},
		{
			name: "engine count changed - invalid",
			current: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
			},
			updated: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 5, Concurrency: 10}, // Changed engines
			},
			expectInvalid: true,
			expectedMsg:   "You cannot change engine numbers while having engines deployed",
		},
		{
			name: "concurrency changed - invalid",
			current: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
			},
			updated: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 20}, // Changed concurrency
			},
			expectInvalid: true,
			expectedMsg:   "You cannot change concurrency while having engines deployed",
		},
		{
			name: "multiple changes - engine count priority",
			current: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10},
			},
			updated: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 5, Concurrency: 20}, // Both changed, but engines checked first
			},
			expectInvalid: true,
			expectedMsg:   "You cannot change engine numbers while having engines deployed",
		},
		{
			name: "allowed changes - duration, rampup, name",
			current: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10, Duration: 60, Rampup: 5, Name: "test1"},
			},
			updated: []*model.ExecutionPlan{
				{PlanID: 1, Engines: 2, Concurrency: 10, Duration: 120, Rampup: 10, Name: "test1-updated"},
			},
			expectInvalid: false,
			expectedMsg:   "",
		},
		{
			name:          "empty current and updated - valid",
			current:       []*model.ExecutionPlan{},
			updated:       []*model.ExecutionPlan{},
			expectInvalid: false,
			expectedMsg:   "",
		},
		{
			name:          "nil current and empty updated - valid",
			current:       nil,
			updated:       []*model.ExecutionPlan{},
			expectInvalid: false,
			expectedMsg:   "",
		},
		{
			name:          "empty current and nil updated - valid",
			current:       []*model.ExecutionPlan{},
			updated:       nil,
			expectInvalid: false,
			expectedMsg:   "",
		},
		{
			name:          "both nil - valid",
			current:       nil,
			updated:       nil,
			expectInvalid: false,
			expectedMsg:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isInvalid, msg := hasInvalidDiff(tc.current, tc.updated)
			assert.Equal(t, tc.expectInvalid, isInvalid)
			if tc.expectInvalid {
				assert.Equal(t, tc.expectedMsg, msg)
			} else {
				assert.Empty(t, msg)
			}
		})
	}
}

func TestHasInvalidDiffComplexScenarios(t *testing.T) {
	t.Run("multiple plans with mixed valid/invalid changes", func(t *testing.T) {
		current := []*model.ExecutionPlan{
			{PlanID: 1, Engines: 2, Concurrency: 10},
			{PlanID: 2, Engines: 3, Concurrency: 15},
			{PlanID: 3, Engines: 1, Concurrency: 5},
		}
		updated := []*model.ExecutionPlan{
			{PlanID: 1, Engines: 2, Concurrency: 10}, // No change
			{PlanID: 2, Engines: 5, Concurrency: 15}, // Engine count changed - should fail
			{PlanID: 3, Engines: 1, Concurrency: 5},  // No change
		}

		isInvalid, msg := hasInvalidDiff(current, updated)
		assert.True(t, isInvalid)
		assert.Equal(t, "You cannot change engine numbers while having engines deployed", msg)
	})

	t.Run("large number of plans", func(t *testing.T) {
		var current, updated []*model.ExecutionPlan

		// Create 100 plans
		for i := int64(1); i <= 100; i++ {
			current = append(current, &model.ExecutionPlan{
				PlanID:      i,
				Engines:     2,
				Concurrency: 10,
			})
			updated = append(updated, &model.ExecutionPlan{
				PlanID:      i,
				Engines:     2,
				Concurrency: 10,
			})
		}

		isInvalid, msg := hasInvalidDiff(current, updated)
		assert.False(t, isInvalid)
		assert.Empty(t, msg)
	})

	t.Run("duplicate plan IDs in current", func(t *testing.T) {
		current := []*model.ExecutionPlan{
			{PlanID: 1, Engines: 2, Concurrency: 10},
			{PlanID: 1, Engines: 3, Concurrency: 15}, // Duplicate PlanID - last one wins in map
		}
		updated := []*model.ExecutionPlan{
			{PlanID: 1, Engines: 2, Concurrency: 10}, // Will be compared against engines: 3, concurrency: 15
			{PlanID: 1, Engines: 3, Concurrency: 15},
		}

		// Function should detect engine mismatch (2 vs 3)
		isInvalid, msg := hasInvalidDiff(current, updated)
		assert.True(t, isInvalid)
		assert.Equal(t, "You cannot change engine numbers while having engines deployed", msg)
	})

	t.Run("zero values", func(t *testing.T) {
		current := []*model.ExecutionPlan{
			{PlanID: 0, Engines: 0, Concurrency: 0},
		}
		updated := []*model.ExecutionPlan{
			{PlanID: 0, Engines: 0, Concurrency: 0},
		}

		isInvalid, msg := hasInvalidDiff(current, updated)
		assert.False(t, isInvalid)
		assert.Empty(t, msg)
	})

	t.Run("negative values", func(t *testing.T) {
		current := []*model.ExecutionPlan{
			{PlanID: -1, Engines: -2, Concurrency: -10},
		}
		updated := []*model.ExecutionPlan{
			{PlanID: -1, Engines: -2, Concurrency: -10},
		}

		isInvalid, msg := hasInvalidDiff(current, updated)
		assert.False(t, isInvalid)
		assert.Empty(t, msg)
	})
}

func TestHasInvalidDiffEdgeCases(t *testing.T) {
	t.Run("order doesn't matter", func(t *testing.T) {
		current := []*model.ExecutionPlan{
			{PlanID: 1, Engines: 2, Concurrency: 10},
			{PlanID: 2, Engines: 3, Concurrency: 15},
		}
		updated := []*model.ExecutionPlan{
			{PlanID: 2, Engines: 3, Concurrency: 15}, // Different order
			{PlanID: 1, Engines: 2, Concurrency: 10},
		}

		isInvalid, msg := hasInvalidDiff(current, updated)
		assert.False(t, isInvalid)
		assert.Empty(t, msg)
	})

	t.Run("checks stop at first invalid difference", func(t *testing.T) {
		current := []*model.ExecutionPlan{
			{PlanID: 1, Engines: 2, Concurrency: 10},
			{PlanID: 2, Engines: 3, Concurrency: 15},
		}
		updated := []*model.ExecutionPlan{
			{PlanID: 1, Engines: 5, Concurrency: 20}, // Both engines and concurrency changed
			{PlanID: 2, Engines: 3, Concurrency: 15},
		}

		isInvalid, msg := hasInvalidDiff(current, updated)
		assert.True(t, isInvalid)
		// Should return engines error (checked first)
		assert.Equal(t, "You cannot change engine numbers while having engines deployed", msg)
	})
}

// Test API helper functions

func TestSetagayaAPI_makeRespMessage(t *testing.T) {
	api := &SetagayaAPI{}

	testCases := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "simple message",
			message:  "Operation successful",
			expected: "Operation successful",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
		{
			name:     "message with special characters",
			message:  "Error: failed with status 404 & message=\"not found\"",
			expected: "Error: failed with status 404 & message=\"not found\"",
		},
		{
			name:     "long message",
			message:  strings.Repeat("test ", 100),
			expected: strings.Repeat("test ", 100),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := api.makeRespMessage(tc.message)
			assert.NotNil(t, result)
			assert.Equal(t, tc.expected, result.Message)
		})
	}
}

func TestSetagayaAPI_jsonise(t *testing.T) {
	api := &SetagayaAPI{}

	testCases := []struct {
		name           string
		status         int
		content        interface{}
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "success response",
			status:         http.StatusOK,
			content:        map[string]string{"message": "success"},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"success"}`,
		},
		{
			name:           "error response",
			status:         http.StatusBadRequest,
			content:        map[string]string{"error": "bad request"},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"bad request"}`,
		},
		{
			name:           "nil content",
			status:         http.StatusNoContent,
			content:        nil,
			expectedStatus: http.StatusNoContent,
			expectedBody:   "null",
		},
		{
			name:           "JSONMessage struct",
			status:         http.StatusCreated,
			content:        &JSONMessage{Message: "created"},
			expectedStatus: http.StatusCreated,
			expectedBody:   `{"message":"created"}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			api.jsonise(w, tc.status, tc.content)

			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			// Parse and compare JSON to handle formatting differences
			var expected, actual interface{}
			json.Unmarshal([]byte(tc.expectedBody), &expected)
			json.Unmarshal(w.Body.Bytes(), &actual)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestSetagayaAPI_makeFailMessage(t *testing.T) {
	api := &SetagayaAPI{}

	testCases := []struct {
		name           string
		message        string
		statusCode     int
		expectedStatus int
	}{
		{
			name:           "bad request",
			message:        "Invalid input",
			statusCode:     http.StatusBadRequest,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "not found",
			message:        "Resource not found",
			statusCode:     http.StatusNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "internal server error",
			message:        "Something went wrong",
			statusCode:     http.StatusInternalServerError,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "empty message",
			message:        "",
			statusCode:     http.StatusBadRequest,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			api.makeFailMessage(w, tc.message, tc.statusCode)

			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var response JSONMessage
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tc.message, response.Message)
		})
	}
}

func TestSetagayaAPI_handleErrorsFromExt(t *testing.T) {
	api := &SetagayaAPI{}

	testCases := []struct {
		name           string
		inputError     error
		expectNil      bool
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "DBError",
			inputError: &model.DBError{
				Err:     errors.New("database error"),
				Message: "Database connection failed",
			},
			expectNil:      true,
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "Database connection failed",
		},
		{
			name: "NoResourcesFoundErr",
			inputError: &scheduler.NoResourcesFoundErr{
				Err:     errors.New("no resources"),
				Message: "No available resources",
			},
			expectNil:      true,
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "No available resources",
		},
		{
			name:       "Unknown error type",
			inputError: errors.New("unknown error"),
			expectNil:  false,
		},
		{
			name:       "Nil error",
			inputError: nil,
			expectNil:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			result := api.handleErrorsFromExt(w, tc.inputError)

			if tc.expectNil {
				assert.Nil(t, result)
				assert.Equal(t, tc.expectedStatus, w.Code)

				var response JSONMessage
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedMsg, response.Message)
			} else {
				assert.Equal(t, tc.inputError, result)
			}
		})
	}
}
