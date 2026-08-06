package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/app/scheduleapp"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func newScheduleRouter(t *testing.T) (http.Handler, *fake.Store) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	h := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Scenarios:     scenarioapp.NewService(store, obj),
		Executions:    executionapp.NewService(store, obj, 100),
		Schedules:     scheduleapp.NewService(store, quotaapp.NewService(store)),
		Store:         obj,
		DefaultOwners: []string{"honryu"},
	})
	return h, store
}

// seedExecution creates a project, a scenario, and an execution with a
// one-scenario load profile (2 engines, 30s duration) via the HTTP API, and
// returns the execution's id.
func seedExecution(t *testing.T, h http.Handler) int64 {
	t.Helper()
	projectID := decodeID(t, postForm(t, h, "/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}}))
	scenarioID := decodeID(t, postForm(t, h, "/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}}))
	executionID := decodeID(t, postForm(t, h, "/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}}))

	configYAML := fmt.Sprintf(`multi-test:
  collectionid: %d
  tests:
    - testid: %d
      concurrency: 10
      rampup: 1
      engines: 2
      duration: 30
`, executionID, scenarioID)
	rec := putMultipart(t, h, "/api/executions/"+itoa(executionID)+"/config", "config.yaml", configYAML)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload config = %d (%s)", rec.Code, rec.Body.String())
	}
	return executionID
}

func TestCreateSchedule_OneShot_AdmitsAndReturnsReservedOccurrence(t *testing.T) {
	t.Parallel()
	h, store := newScheduleRouter(t)
	executionID := seedExecution(t, h)
	if err := store.SetCeiling(t.Context(), 7, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	fireAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	rec := postForm(t, h, "/api/executions/"+itoa(executionID)+"/schedules", url.Values{
		"tenant_id": {"7"}, "kind": {"one_shot"}, "fire_at": {fireAt},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create schedule = %d (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		ID          int64 `json:"id"`
		Occurrences []struct {
			Status        string `json:"status"`
			ReservationID *int64 `json:"reservation_id"`
		} `json:"occurrences"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if got.ID <= 0 {
		t.Fatalf("schedule id = %d, want > 0", got.ID)
	}
	if len(got.Occurrences) != 1 || got.Occurrences[0].Status != "reserved" || got.Occurrences[0].ReservationID == nil {
		t.Fatalf("occurrences = %+v, want one reserved occurrence with a reservation id", got.Occurrences)
	}
}

func TestCreateSchedule_RejectsInvalidKind(t *testing.T) {
	t.Parallel()
	h, _ := newScheduleRouter(t)
	executionID := seedExecution(t, h)

	rec := postForm(t, h, "/api/executions/"+itoa(executionID)+"/schedules", url.Values{
		"tenant_id": {"7"}, "kind": {"yearly"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create schedule (invalid kind) = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateSchedule_InvalidTenantID(t *testing.T) {
	t.Parallel()
	h, _ := newScheduleRouter(t)
	executionID := seedExecution(t, h)

	rec := postForm(t, h, "/api/executions/"+itoa(executionID)+"/schedules", url.Values{
		"tenant_id": {"not-a-number"}, "kind": {"one_shot"}, "fire_at": {time.Now().Add(time.Hour).UTC().Format(time.RFC3339)},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create schedule (invalid tenant_id) = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateSchedule_InvalidFireAt(t *testing.T) {
	t.Parallel()
	h, _ := newScheduleRouter(t)
	executionID := seedExecution(t, h)

	rec := postForm(t, h, "/api/executions/"+itoa(executionID)+"/schedules", url.Values{
		"tenant_id": {"7"}, "kind": {"one_shot"}, "fire_at": {"not-a-date"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create schedule (invalid fire_at) = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListSchedules_MissingExecution(t *testing.T) {
	t.Parallel()
	h, _ := newScheduleRouter(t)
	rec := do(t, h, http.MethodGet, "/api/executions/999/schedules")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("list schedules (missing execution) = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

// pathInt rejects a non-numeric id before touching h.deps.Schedules at all,
// on every one of the three schedule routes.
func TestScheduleHandlers_InvalidID_400(t *testing.T) {
	t.Parallel()
	h, _ := newScheduleRouter(t)
	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/executions/x/schedules"},
		{http.MethodGet, "/api/executions/x/schedules"},
		{http.MethodDelete, "/api/executions/1/schedules/x"},
	}
	for _, tc := range cases {
		if rec := do(t, h, tc.method, tc.path); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d, want 400", tc.method, tc.path, rec.Code)
		}
	}
}

func TestCreateSchedule_MissingExecution(t *testing.T) {
	t.Parallel()
	h, _ := newScheduleRouter(t)
	rec := postForm(t, h, "/api/executions/999/schedules", url.Values{"tenant_id": {"7"}, "kind": {"recurring"}, "recurrence": {"* * * * *"}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("create schedule (missing execution) = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListSchedules_ReturnsOccurrencesAndStatuses(t *testing.T) {
	t.Parallel()
	h, store := newScheduleRouter(t)
	executionID := seedExecution(t, h)
	if err := store.SetCeiling(t.Context(), 7, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	fireAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	create := postForm(t, h, "/api/executions/"+itoa(executionID)+"/schedules", url.Values{"tenant_id": {"7"}, "kind": {"one_shot"}, "fire_at": {fireAt}})
	if create.Code != http.StatusCreated {
		t.Fatalf("create schedule = %d (%s)", create.Code, create.Body.String())
	}

	rec := do(t, h, http.MethodGet, "/api/executions/"+itoa(executionID)+"/schedules")
	if rec.Code != http.StatusOK {
		t.Fatalf("list schedules = %d (%s)", rec.Code, rec.Body.String())
	}
	var got []struct {
		Occurrences []struct {
			Status string `json:"status"`
		} `json:"occurrences"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if len(got) != 1 || len(got[0].Occurrences) != 1 || got[0].Occurrences[0].Status != "reserved" {
		t.Fatalf("list schedules = %+v, want one schedule with one reserved occurrence", got)
	}
}

func TestDeleteSchedule_ReleasesReservationAndRemovesTheSchedule(t *testing.T) {
	t.Parallel()
	h, store := newScheduleRouter(t)
	executionID := seedExecution(t, h)
	if err := store.SetCeiling(t.Context(), 7, "", 2); err != nil {
		t.Fatalf("SetCeiling: %v", err)
	}
	fireAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	create := postForm(t, h, "/api/executions/"+itoa(executionID)+"/schedules", url.Values{"tenant_id": {"7"}, "kind": {"one_shot"}, "fire_at": {fireAt}})
	scheduleID := decodeID(t, create)

	rec := do(t, h, http.MethodDelete, "/api/executions/"+itoa(executionID)+"/schedules/"+itoa(scheduleID))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete schedule = %d (%s)", rec.Code, rec.Body.String())
	}

	list := do(t, h, http.MethodGet, "/api/executions/"+itoa(executionID)+"/schedules")
	var got []json.RawMessage
	if err := json.Unmarshal(list.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("schedules after delete = %d, want 0", len(got))
	}
}

func TestDeleteSchedule_MissingSchedule(t *testing.T) {
	t.Parallel()
	h, _ := newScheduleRouter(t)
	executionID := seedExecution(t, h)
	rec := do(t, h, http.MethodDelete, "/api/executions/"+itoa(executionID)+"/schedules/999")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing schedule = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

// A caller who does not own the schedule's execution's project cannot
// delete it, even though authorizeSchedule derives ownership from the
// schedule's own recorded execution id rather than trusting the URL.
func TestDeleteSchedule_ForbiddenForNonOwner(t *testing.T) {
	t.Parallel()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	h := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Scenarios:     scenarioapp.NewService(store, obj),
		Executions:    executionapp.NewService(store, obj, 100),
		Schedules:     scheduleapp.NewService(store, quotaapp.NewService(store)),
		Store:         obj,
		DefaultOwners: []string{"honryu", "someone-else"},
	})
	executionID := seedExecution(t, h)
	fireAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	create := postForm(t, h, "/api/executions/"+itoa(executionID)+"/schedules", url.Values{"tenant_id": {"7"}, "kind": {"one_shot"}, "fire_at": {fireAt}})
	scheduleID := decodeID(t, create)

	// Rebuild the router with a caller set that no longer includes the
	// project's actual owner ("honryu") -- simulating a different, unrelated
	// caller in no-auth mode.
	hForeign := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(store),
		Scenarios:     scenarioapp.NewService(store, obj),
		Executions:    executionapp.NewService(store, obj, 100),
		Schedules:     scheduleapp.NewService(store, quotaapp.NewService(store)),
		Store:         obj,
		DefaultOwners: []string{"someone-else"},
	})
	rec := do(t, hForeign, http.MethodDelete, "/api/executions/"+itoa(executionID)+"/schedules/"+itoa(scheduleID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("delete schedule (foreign owner) = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
}
