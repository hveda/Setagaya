package scenarioapp_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func newScenarioService(t *testing.T) (*scenarioapp.Service, *fake.Store, *fake.ObjectStore) {
	t.Helper()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	return scenarioapp.NewService(store, obj), store, obj
}

func TestCreate_Get_List(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()

	p, err := svc.Create(ctx, "smoke", 10)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("Create returned zero ID")
	}
	got, err := svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "smoke" {
		t.Fatalf("Get name = %q, want smoke", got.Name)
	}
	list, err := svc.ListByProject(ctx, 10)
	if err != nil {
		t.Fatalf("ListByProject: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListByProject len = %d, want 1", len(list))
	}
}

func TestCreate_InvalidName(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	if _, err := svc.Create(context.Background(), "", 10); err == nil {
		t.Fatal("Create with empty name: expected error")
	}
}

func TestFileLifecycle(t *testing.T) {
	t.Parallel()
	svc, _, obj := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)

	// Upload a JMX (test file) and a CSV (data file).
	if err := svc.UploadFile(ctx, p.ID, "scenario.jmx", bytes.NewReader([]byte("<jmx/>"))); err != nil {
		t.Fatalf("UploadFile jmx: %v", err)
	}
	if err := svc.UploadFile(ctx, p.ID, "users.csv", bytes.NewReader([]byte("a,b"))); err != nil {
		t.Fatalf("UploadFile csv: %v", err)
	}

	// The object store holds them under the scenario key convention.
	if _, err := obj.Download(ctx, "scenario/1/scenario.jmx"); err != nil {
		t.Fatalf("object not stored at scenario/1/scenario.jmx: %v", err)
	}

	files, err := svc.Files(ctx, p.ID)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if files.TestFile == nil || files.TestFile.Filename != "scenario.jmx" {
		t.Fatalf("TestFile = %+v, want scenario.jmx", files.TestFile)
	}
	if len(files.Data) != 1 || files.Data[0].Filename != "users.csv" || files.Data[0].URL == "" {
		t.Fatalf("Data = %+v, want [users.csv] with URL", files.Data)
	}

	// Download round trip.
	got, err := svc.DownloadFile(ctx, p.ID, "users.csv")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if string(got) != "a,b" {
		t.Fatalf("DownloadFile = %q, want a,b", got)
	}

	// Duplicate upload is rejected.
	if err := svc.UploadFile(ctx, p.ID, "users.csv", bytes.NewReader([]byte("x"))); !errors.Is(err, ports.ErrFileExists) {
		t.Fatalf("duplicate UploadFile = %v, want ErrFileExists", err)
	}

	// Delete the data file; it disappears from storage and listing.
	if err := svc.DeleteFile(ctx, p.ID, "users.csv"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, err := obj.Download(ctx, "scenario/1/users.csv"); !errors.Is(err, ports.ErrObjectNotFound) {
		t.Fatalf("object still present after delete: %v", err)
	}
}

func TestSetRequests_ValidFragmentPersists(t *testing.T) {
	t.Parallel()
	svc, store, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "portable", 10)

	raw := []byte("default-address: http://example.com\nrequests:\n  - url: /checkout\n")
	if err := svc.SetRequests(ctx, p.ID, raw); err != nil {
		t.Fatalf("SetRequests: %v", err)
	}

	got, err := store.GetScenarioRequests(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetScenarioRequests: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("stored requests = %q, want %q (stored exactly as uploaded, not re-marshaled)", got, raw)
	}
}

// A scenario pinned native by an uploaded script has nothing that ever reads
// stored requests (compileShards only checks for a portable scenario), so
// accepting an upload here would silently store data nothing will ever use.
func TestSetRequests_RejectsNativeScenario(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "will-be-native", 10)
	if err := svc.UploadFile(ctx, p.ID, "scenario.jmx", strings.NewReader("<jmx/>")); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}

	raw := []byte("requests:\n  - url: /checkout\n")
	if err := svc.SetRequests(ctx, p.ID, raw); !errors.Is(err, scenarioapp.ErrScenarioNotPortable) {
		t.Fatalf("SetRequests(native scenario) = %v, want ErrScenarioNotPortable", err)
	}
}

func TestSetRequests_UnknownScenario(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	raw := []byte("requests:\n  - url: /checkout\n")
	if err := svc.SetRequests(context.Background(), 999, raw); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("SetRequests(unknown scenario) = %v, want ErrNotFound", err)
	}
}

func TestSetRequests_RejectsMalformedYAML(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "portable", 10)

	if err := svc.SetRequests(ctx, p.ID, []byte("not: [valid: yaml")); !errors.Is(err, scenarioapp.ErrRequestsInvalid) {
		t.Fatalf("SetRequests(malformed YAML) = %v, want ErrRequestsInvalid", err)
	}
}

func TestSetRequests_RejectsEmptyRequests(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "portable", 10)

	// Valid YAML, but no requests -- exactly what would otherwise reach
	// compile.Taurus and only fail there as ErrRequestsRequired, with a worse
	// error at the wrong layer.
	if err := svc.SetRequests(ctx, p.ID, []byte("default-address: http://example.com\n")); !errors.Is(err, scenarioapp.ErrRequestsInvalid) {
		t.Fatalf("SetRequests(no requests) = %v, want ErrRequestsInvalid", err)
	}
}

func TestSetRequests_RejectsRequestWithNoURL(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "portable", 10)

	if err := svc.SetRequests(ctx, p.ID, []byte("requests:\n  - method: GET\n")); !errors.Is(err, scenarioapp.ErrRequestsInvalid) {
		t.Fatalf("SetRequests(request with no url) = %v, want ErrRequestsInvalid", err)
	}
}

// A later upload overwrites, never merges with, an earlier one -- PUT
// semantics, matching the persistence layer's own contract.
func TestSetRequests_LaterUploadOverwrites(t *testing.T) {
	t.Parallel()
	svc, store, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "portable", 10)

	if err := svc.SetRequests(ctx, p.ID, []byte("requests:\n  - url: /one\n")); err != nil {
		t.Fatalf("SetRequests (first): %v", err)
	}
	second := []byte("requests:\n  - url: /two\n")
	if err := svc.SetRequests(ctx, p.ID, second); err != nil {
		t.Fatalf("SetRequests (second): %v", err)
	}

	got, err := store.GetScenarioRequests(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetScenarioRequests: %v", err)
	}
	if string(got) != string(second) {
		t.Errorf("stored requests = %q, want %q (the second upload alone)", got, second)
	}
}

func TestUploadFile_InvalidFilename(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)

	for _, name := range []string{"", "sub/dir.csv", "..", "."} {
		if err := svc.UploadFile(ctx, p.ID, name, bytes.NewReader(nil)); !errors.Is(err, scenarioapp.ErrInvalidFilename) {
			t.Fatalf("UploadFile(%q) = %v, want ErrInvalidFilename", name, err)
		}
	}
}

func TestUploadFile_UnknownScenario(t *testing.T) {
	t.Parallel()
	svc, _, _ := newScenarioService(t)
	if err := svc.UploadFile(context.Background(), 999, "a.csv", bytes.NewReader(nil)); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("UploadFile(unknown scenario) = %v, want ErrNotFound", err)
	}
}

func TestDelete_RefusesWhenInUse(t *testing.T) {
	t.Parallel()
	svc, store, _ := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)

	coll, _ := execution.New("peak", 10)
	collID, _ := store.CreateExecution(ctx, coll)
	if err := store.StoreLoadProfile(ctx, collID, false, []loadprofile.Entry{
		{ScenarioID: p.ID, Engines: 1, Concurrency: 1, Duration: 60},
	}); err != nil {
		t.Fatalf("seed execution: %v", err)
	}

	if err := svc.Delete(ctx, p.ID); !errors.Is(err, scenarioapp.ErrScenarioInUse) {
		t.Fatalf("Delete(in use) = %v, want ErrScenarioInUse", err)
	}
}

func TestDelete_RemovesFiles(t *testing.T) {
	t.Parallel()
	svc, _, obj := newScenarioService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, "smoke", 10)
	_ = svc.UploadFile(ctx, p.ID, "scenario.jmx", bytes.NewReader([]byte("x")))

	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := obj.Download(ctx, "scenario/1/scenario.jmx"); !errors.Is(err, ports.ErrObjectNotFound) {
		t.Fatalf("file survived scenario delete: %v", err)
	}
	if _, err := svc.Get(ctx, p.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("scenario survived delete: %v", err)
	}
}

// Uploading an engine-native script decides what the scenario is. Without this a
// scenario stays portable with no requests, which nothing can compile -- so the
// upload would appear to succeed and the deploy would fail later.
func TestUploadFile_PinsTheScenarioToItsEngine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		filename string
		wantKind scenario.Kind
		engine   taurus.Executor
	}{
		{"plan.jmx", scenario.KindNative, taurus.ExecutorJMeter},
		{"load.js", scenario.KindNative, taurus.ExecutorK6},
		// Data files say nothing about which engine runs the scenario.
		{"users.csv", scenario.KindPortable, ""},
	}
	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			store := fake.NewStore()
			svc := scenarioapp.NewService(store, fake.NewObjectStore())

			sc, err := svc.Create(ctx, "probe", 1)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if err := svc.UploadFile(ctx, sc.ID, tc.filename, strings.NewReader("x")); err != nil {
				t.Fatalf("UploadFile: %v", err)
			}

			got, err := svc.Get(ctx, sc.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got.Kind != tc.wantKind || got.Engine != tc.engine {
				t.Errorf("after uploading %s: kind %q engine %q, want %q/%q",
					tc.filename, got.Kind, got.Engine, tc.wantKind, tc.engine)
			}
		})
	}
}

// A scenario pinned by its script must be able to say where it can run.
func TestUploadFile_PinnedScenarioRefusesAnotherEngine(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	svc := scenarioapp.NewService(store, fake.NewObjectStore())

	sc, _ := svc.Create(ctx, "probe", 1)
	if err := svc.UploadFile(ctx, sc.ID, "plan.jmx", strings.NewReader("<jmx/>")); err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	got, _ := svc.Get(ctx, sc.ID)

	if err := got.CanRunOn(taurus.ExecutorJMeter); err != nil {
		t.Errorf("CanRunOn(jmeter) = %v, want nil", err)
	}
	if err := got.CanRunOn(taurus.ExecutorK6); err == nil {
		t.Error("a JMeter plan accepted k6")
	}
}

func TestImportJMX(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := fake.NewStore()
	obj := fake.NewObjectStore()
	svc := scenarioapp.NewService(store, obj)

	plan := `<?xml version="1.0"?><jmeterTestPlan><hashTree>
	  <TestPlan testname="Checkout"/>
	  <CSVDataSet testname="a" enabled="true"><stringProp name="filename">/abs/users.csv</stringProp></CSVDataSet>
	</hashTree></jmeterTestPlan>`

	res, err := svc.ImportJMX(ctx, "checkout", 1, "checkout.jmx", strings.NewReader(plan))
	if err != nil {
		t.Fatalf("ImportJMX: %v", err)
	}
	if res.Scenario.Kind != scenario.KindNative || res.Scenario.Engine != taurus.ExecutorJMeter {
		t.Errorf("imported scenario = kind %q engine %q", res.Scenario.Kind, res.Scenario.Engine)
	}
	if res.Report.TestPlanName != "Checkout" || len(res.Report.Findings) == 0 {
		t.Errorf("report = %+v", res.Report)
	}
	// The plan itself must be stored, or the scenario cannot run.
	files, err := svc.Files(ctx, res.Scenario.ID)
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if files.TestFile == nil || files.TestFile.Filename != "checkout.jmx" {
		t.Errorf("stored files = %+v", files)
	}
}

func TestImportJMX_Rejections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := scenarioapp.NewService(fake.NewStore(), fake.NewObjectStore())

	cases := []struct{ name, filename, body string }{
		{"not a jmx file", "plan.txt", "<jmeterTestPlan/>"},
		{"malformed xml", "plan.jmx", "<jmeterTestPlan>"},
		{"not a test plan", "plan.jmx", `<project/>`},
		{"no TestPlan element", "plan.jmx", `<jmeterTestPlan><hashTree><WorkBench/></hashTree></jmeterTestPlan>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := svc.ImportJMX(ctx, "x", 1, tc.filename, strings.NewReader(tc.body)); err == nil {
				t.Errorf("ImportJMX(%s) succeeded, want an error", tc.name)
			}
		})
	}
}
