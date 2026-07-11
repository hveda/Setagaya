//go:build e2e

package e2e_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	membus "github.com/heridotlife/Setagaya/internal/adapters/eventbus/memory"
	"github.com/heridotlife/Setagaya/internal/adapters/httpapi"
	mysqladapter "github.com/heridotlife/Setagaya/internal/adapters/repo/mysql"
	"github.com/heridotlife/Setagaya/internal/adapters/storage/local"
	"github.com/heridotlife/Setagaya/internal/app/adminapp"
	"github.com/heridotlife/Setagaya/internal/app/collectionapp"
	"github.com/heridotlife/Setagaya/internal/app/lifecycleapp"
	"github.com/heridotlife/Setagaya/internal/app/metricsapp"
	"github.com/heridotlife/Setagaya/internal/app/planapp"
	"github.com/heridotlife/Setagaya/internal/app/projectapp"
	"github.com/heridotlife/Setagaya/internal/app/usageapp"
	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
	"github.com/heridotlife/Setagaya/test/dbtest"
)

// TestPhase3_MetricsUsageAdminEndToEnd drives a run and asserts the live SSE
// metric stream, usage accounting, and admin listing over real HTTP with a real
// MySQL container. Scheduler/executor are fakes (the executor replays metrics).
func TestPhase3_MetricsUsageAdminEndToEnd(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	store := local.New(t.TempDir(), "")
	sched := fake.NewScheduler()
	exec := fake.NewExecutor()
	exec.Repeat = true
	exec.Metrics = []engine.Metric{{Label: "home", Status: "200", Latency: 12.5, Threads: 8}}
	sink := fake.NewMetricsSink()
	bus := membus.New()

	collector := metricsapp.NewService(repo, sched, exec, sink, bus)
	usage := usageapp.NewService(repo)
	lifecycle := lifecycleapp.NewService(repo, sched, exec, store, "jmeter").WithMetrics(collector).WithUsage(usage)
	admin := adminapp.NewService(repo, sched, lifecycle)

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		Plans:         planapp.NewService(repo, store),
		Collections:   collectionapp.NewService(repo, store, 500),
		Lifecycle:     lifecycle,
		Usage:         usage,
		Admin:         admin,
		Events:        bus,
		Store:         store,
		DefaultOwners: []string{"setagaya"},
	})
	srv := httptest.NewServer(router)
	defer srv.Close()
	client := srv.Client()

	projectID := postForm(t, client, srv.URL+"/api/projects", url.Values{"name": {"web"}, "owner": {"setagaya"}})
	planID := postForm(t, client, srv.URL+"/api/plans", url.Values{"name": {"smoke"}, "project_id": {itoa(projectID)}})
	putMultipart(t, client, srv.URL+"/api/plans/"+itoa(planID)+"/files", "plan.jmx", "<jmx/>")
	collID := postForm(t, client, srv.URL+"/api/collections", url.Values{"name": {"peak"}, "project_id": {itoa(projectID)}})
	cfg := fmt.Sprintf("multi-test:\n  collectionid: %d\n  tests:\n    - testid: %d\n      concurrency: 10\n      rampup: 1\n      engines: 2\n      duration: 30\n", collID, planID)
	putMultipart(t, client, srv.URL+"/api/collections/"+itoa(collID)+"/config", "config.yaml", cfg)

	base := srv.URL + "/api/collections/" + itoa(collID)
	postAction(t, client, base+"/deploy", http.StatusOK)
	postAction(t, client, base+"/trigger", http.StatusOK)

	// The live SSE stream delivers enriched metrics during the run.
	streamCtx, cancelStream := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, base+"/stream", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	line := readFirstSSE(t, resp.Body)
	if !strings.Contains(line, `"label":"home"`) || !strings.Contains(line, `"collection_id":"`+itoa(collID)+`"`) {
		t.Fatalf("SSE metric = %q", line)
	}
	cancelStream()
	_ = resp.Body.Close()

	// Admin lists the collection as deployed.
	var running []adminapp.RunningCollection
	getJSON(t, client, srv.URL+"/api/admin/collections", http.StatusOK, &running)
	if len(running) != 1 || running[0].CollectionID != collID {
		t.Fatalf("admin collections = %+v", running)
	}

	postAction(t, client, base+"/stop", http.StatusOK)

	// Usage history now has one finished launch for this collection.
	var history []map[string]any
	getJSON(t, client, srv.URL+"/api/usage/history", http.StatusOK, &history)
	found := false
	for _, h := range history {
		if int64(h["CollectionID"].(float64)) == collID {
			found = true
		}
	}
	if !found {
		t.Fatalf("usage history missing collection %d: %+v", collID, history)
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
