//go:build e2e

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	membus "github.com/heridotlife/honryu/internal/adapters/eventbus/memory"
	"github.com/heridotlife/honryu/internal/adapters/httpapi"
	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/adapters/storage/local"
	"github.com/heridotlife/honryu/internal/app/adminapp"
	"github.com/heridotlife/honryu/internal/app/executionapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/metricsapp"
	"github.com/heridotlife/honryu/internal/app/projectapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/app/usageapp"
	"github.com/heridotlife/honryu/internal/domain/metrics"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/test/dbtest"
)

// TestPhase3_MetricsUsageAdminEndToEnd drives a run and asserts the live SSE
// metric stream, usage accounting, and admin listing over real HTTP with a real
// MySQL container. The scheduler is a fake, and measurements are pushed through
// the ingest seam the sidecar will use.
func TestPhase3_MetricsUsageAdminEndToEnd(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()
	sink := fake.NewMetricsSink()
	bus := membus.New()

	collector := metricsapp.NewService(repo, sink, bus)
	usage := usageapp.NewService(repo)
	lifecycle := lifecycleapp.NewService(repo, sched, store, lifecycleapp.StaticImage("jmeter")).WithMetrics(collector).WithUsage(usage)
	admin := adminapp.NewService(repo, sched, lifecycle)

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		Scenarios:     scenarioapp.NewService(repo, store),
		Executions:    executionapp.NewService(repo, store, 500),
		Lifecycle:     lifecycle,
		Usage:         usage,
		Admin:         admin,
		Events:        bus,
		Store:         store,
		Metrics:       collector,
		IngestToken:   "engine-token",
		DefaultOwners: []string{"honryu"},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()
	client := srv.Client()

	projectID := postForm(t, client, srv.URL+"/api/projects", url.Values{"name": {"web"}, "owner": {"honryu"}})
	scenarioID := postForm(t, client, srv.URL+"/api/scenarios", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}})
	putMultipart(t, client, srv.URL+"/api/scenarios/"+itoa(scenarioID)+"/files", "scenario.jmx", "<jmx/>")
	collID := postForm(t, client, srv.URL+"/api/executions", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	cfg := fmt.Sprintf("multi-test:\n  collectionid: %d\n  tests:\n    - testid: %d\n      concurrency: 10\n      rampup: 1\n      engines: 2\n      duration: 30\n", collID, scenarioID)
	putMultipart(t, client, srv.URL+"/api/executions/"+itoa(collID)+"/config", "config.yaml", cfg)

	base := srv.URL + "/api/executions/" + itoa(collID)
	postAction(t, client, base+"/deploy", http.StatusOK)
	postAction(t, client, base+"/trigger", http.StatusOK)

	// Measurements arrive over the same endpoint a sidecar in an engine pod
	// posts to, so this exercises the whole inbound path rather than reaching
	// past it into the service.
	runID, _, err := repo.CurrentRun(context.Background(), collID)
	if err != nil {
		t.Fatalf("CurrentRun: %v", err)
	}
	pushCtx, stopPushing := context.WithCancel(context.Background())
	defer stopPushing()
	go func() {
		for ts := int64(0); ; ts++ {
			select {
			case <-pushCtx.Done():
				return
			default:
			}
			body, _ := json.Marshal(metrics.Batch{
				ExecutionID: collID, ScenarioID: scenarioID, RunID: runID, ShardIndex: 0,
				Intervals: []metrics.Interval{{
					Timestamp: ts, Label: "home", Concurrency: 8, Samples: 20, Succeeded: 20,
					Latency: metrics.Histogram{0.0125: 20}, ResponseCodes: map[string]int64{"200": 20},
				}},
			})
			req, _ := http.NewRequestWithContext(pushCtx, http.MethodPost,
				srv.URL+"/api/ingest", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer engine-token")
			if resp, err := client.Do(req); err == nil {
				_ = resp.Body.Close()
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// The live SSE stream delivers enriched metrics during the run.
	streamCtx, cancelStream := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, base+"/stream", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	line := readFirstSSE(t, resp.Body)
	if !strings.Contains(line, `"label":"home"`) || !strings.Contains(line, `"execution_id":"`+itoa(collID)+`"`) {
		t.Fatalf("SSE metric = %q", line)
	}
	cancelStream()
	_ = resp.Body.Close()

	// Admin lists the execution as deployed.
	var running []adminapp.RunningExecution
	getJSON(t, client, srv.URL+"/api/admin/executions", http.StatusOK, &running)
	if len(running) != 1 || running[0].ExecutionID != collID {
		t.Fatalf("admin executions = %+v", running)
	}

	postAction(t, client, base+"/stop", http.StatusOK)

	// Usage history now has one finished launch for this execution.
	var history []map[string]any
	getJSON(t, client, srv.URL+"/api/usage/history", http.StatusOK, &history)
	found := false
	for _, h := range history {
		if int64(h["ExecutionID"].(float64)) == collID {
			found = true
		}
	}
	if !found {
		t.Fatalf("usage history missing execution %d: %+v", collID, history)
	}

	postAction(t, client, base+"/purge", http.StatusOK)
}

func readFirstSSE(t *testing.T, r interface{ Read([]byte) (int, error) }) string {
	t.Helper()
	out := make(chan string, 1)
	go func() {
		br := bufio.NewReader(r)
		for {
			s, err := br.ReadString('\n')
			if data, ok := strings.CutPrefix(s, "data: "); ok {
				out <- strings.TrimSpace(data)
				return
			}
			if err != nil {
				out <- ""
				return
			}
		}
	}()
	select {
	case s := <-out:
		if s == "" {
			t.Fatal("no SSE data received")
		}
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE data")
		return ""
	}
}
