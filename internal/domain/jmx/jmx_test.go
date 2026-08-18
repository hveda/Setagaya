package jmx_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/jmx"
)

const minimalPlan = `<?xml version="1.0" encoding="UTF-8"?>
<jmeterTestPlan version="1.2" properties="5.0" jmeter="5.6.3">
  <hashTree>
    <TestPlan guiclass="TestPlanGui" testclass="TestPlan" testname="Checkout journey" enabled="true"/>
    <hashTree>
      <ThreadGroup guiclass="ThreadGroupGui" testclass="ThreadGroup" testname="Shoppers" enabled="true">
        <stringProp name="ThreadGroup.num_threads">50</stringProp>
        <stringProp name="ThreadGroup.ramp_time">30</stringProp>
        <stringProp name="ThreadGroup.duration">600</stringProp>
      </ThreadGroup>
      <hashTree/>
    </hashTree>
  </hashTree>
</jmeterTestPlan>`

func TestInspect_ReadsPlanAndThreadGroups(t *testing.T) {
	t.Parallel()

	rep, err := jmx.Inspect(strings.NewReader(minimalPlan))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if rep.TestPlanName != "Checkout journey" {
		t.Errorf("TestPlanName = %q", rep.TestPlanName)
	}
	if len(rep.ThreadGroups) != 1 {
		t.Fatalf("ThreadGroups = %d, want 1", len(rep.ThreadGroups))
	}
	tg := rep.ThreadGroups[0]
	if tg.Name != "Shoppers" || tg.Threads != 50 || tg.RampUpSeconds != 30 || tg.DurationSeconds != 600 {
		t.Errorf("thread group = %+v", tg)
	}
}

// Honryu drives concurrency and duration from the execution's load profile, so a
// JMX that hardcodes them will not run as its author wrote it. Silently
// overriding would be the worst outcome: the run would be valid but not the one
// the service owner thought they scheduled.
func TestInspect_ReportsOverriddenLoadSettings(t *testing.T) {
	t.Parallel()

	rep, err := jmx.Inspect(strings.NewReader(minimalPlan))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	f := findingOfKind(t, rep, jmx.FindingLoadOverridden)
	for _, want := range []string{"Shoppers", "50"} {
		if !strings.Contains(f.Detail, want) {
			t.Errorf("finding %q does not mention %q", f.Detail, want)
		}
	}
}

func TestInspect_CollectsDataFilesAndFlagsAbsolutePaths(t *testing.T) {
	t.Parallel()

	plan := `<?xml version="1.0"?>
<jmeterTestPlan>
  <hashTree>
    <TestPlan testname="P"/>
    <CSVDataSet testname="users" enabled="true">
      <stringProp name="filename">users.csv</stringProp>
    </CSVDataSet>
    <CSVDataSet testname="absolute" enabled="true">
      <stringProp name="filename">/var/data/prod-accounts.csv</stringProp>
    </CSVDataSet>
    <CSVDataSet testname="parent" enabled="true">
      <stringProp name="filename">../shared/tokens.csv</stringProp>
    </CSVDataSet>
  </hashTree>
</jmeterTestPlan>`

	rep, err := jmx.Inspect(strings.NewReader(plan))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(rep.DataFiles) != 3 {
		t.Fatalf("DataFiles = %v, want 3", rep.DataFiles)
	}

	var flagged []string
	for _, f := range rep.Findings {
		if f.Kind == jmx.FindingUnreachablePath {
			flagged = append(flagged, f.Detail)
		}
	}
	if len(flagged) != 2 {
		t.Fatalf("unreachable-path findings = %v, want 2 (absolute and parent-relative)", flagged)
	}
	joined := strings.Join(flagged, " ")
	for _, want := range []string{"/var/data/prod-accounts.csv", "../shared/tokens.csv"} {
		if !strings.Contains(joined, want) {
			t.Errorf("findings %v do not mention %q", flagged, want)
		}
	}
}

// Listeners that write to disk and backend listeners that ship metrics elsewhere
// are both pointless under Honryu -- results come from bzt and go to the control
// plane -- and a backend listener would quietly send load data to a third party.
func TestInspect_FlagsListeners(t *testing.T) {
	t.Parallel()

	plan := `<?xml version="1.0"?>
<jmeterTestPlan>
  <hashTree>
    <TestPlan testname="P"/>
    <ResultCollector testname="Write results" enabled="true">
      <stringProp name="filename">/tmp/results.jtl</stringProp>
    </ResultCollector>
    <BackendListener testname="Ship to InfluxDB" enabled="true"/>
    <ResultCollector testname="Disabled one" enabled="false"/>
  </hashTree>
</jmeterTestPlan>`

	rep, err := jmx.Inspect(strings.NewReader(plan))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	kinds := map[string]int{}
	for _, f := range rep.Findings {
		kinds[f.Kind]++
	}
	if kinds[jmx.FindingListenerIgnored] != 1 {
		t.Errorf("listener findings = %d, want 1 (the disabled one is not reported)", kinds[jmx.FindingListenerIgnored])
	}
	if kinds[jmx.FindingExternalReporting] != 1 {
		t.Errorf("backend-listener findings = %d, want 1", kinds[jmx.FindingExternalReporting])
	}
}

func TestInspect_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr error
	}{
		{"malformed xml", `<jmeterTestPlan><hashTree>`, jmx.ErrMalformed},
		{"not a test plan", `<?xml version="1.0"?><project><target/></project>`, jmx.ErrNotJMX},
		{"empty", ``, jmx.ErrMalformed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := jmx.Inspect(strings.NewReader(tc.in))
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Inspect(%q) = %v, want %v", tc.name, err, tc.wantErr)
			}
		})
	}
}

// A plan with nothing unusual must import clean; otherwise every import carries
// noise and users stop reading the findings.
func TestInspect_CleanPlanHasOnlyLoadFinding(t *testing.T) {
	t.Parallel()

	plan := `<?xml version="1.0"?>
<jmeterTestPlan>
  <hashTree>
    <TestPlan testname="P"/>
    <CSVDataSet testname="users" enabled="true">
      <stringProp name="filename">users.csv</stringProp>
    </CSVDataSet>
  </hashTree>
</jmeterTestPlan>`

	rep, err := jmx.Inspect(strings.NewReader(plan))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("clean plan produced findings: %+v", rep.Findings)
	}
	if len(rep.DataFiles) != 1 || rep.DataFiles[0] != "users.csv" {
		t.Errorf("DataFiles = %v", rep.DataFiles)
	}
}

func findingOfKind(t *testing.T, rep jmx.Report, kind string) jmx.Finding {
	t.Helper()
	for _, f := range rep.Findings {
		if f.Kind == kind {
			return f
		}
	}
	t.Fatalf("no finding of kind %q in %+v", kind, rep.Findings)
	return jmx.Finding{}
}

// A .jmx can be well-formed and still not be runnable: JMeter writes
// WorkBench-only files (recording proxies) with no TestPlan. Importing one would
// create a scenario that fails when a pod starts, so it is refused up front.
// Found by running the inspector over the plans JMeter itself ships.
func TestInspect_RefusesPlanWithoutTestPlan(t *testing.T) {
	t.Parallel()

	workbenchOnly := `<?xml version="1.0"?>
<jmeterTestPlan version="1.2">
  <hashTree>
    <WorkBench guiclass="WorkBenchGui" testclass="WorkBench" testname="WorkBench" enabled="true"/>
    <hashTree>
      <ProxyControl testname="HTTP Proxy Server" enabled="true"/>
    </hashTree>
  </hashTree>
</jmeterTestPlan>`

	_, err := jmx.Inspect(strings.NewReader(workbenchOnly))
	if !errors.Is(err, jmx.ErrNoTestPlan) {
		t.Fatalf("Inspect(workbench-only) = %v, want ErrNoTestPlan", err)
	}
}

// Real plans are messier than fixtures: Windows paths, non-numeric properties
// left by JMeter variables, disabled CSV sets, and nested elements that must not
// confuse the property reader.
func TestInspect_AwkwardRealWorldShapes(t *testing.T) {
	t.Parallel()

	plan := `<?xml version="1.0"?>
<jmeterTestPlan>
  <hashTree>
    <TestPlan testname="P"/>
    <ThreadGroup testname="Variables everywhere">
      <stringProp name="ThreadGroup.num_threads">${__P(threads,10)}</stringProp>
      <stringProp name="ThreadGroup.ramp_time"></stringProp>
      <elementProp name="ThreadGroup.main_controller" elementType="LoopController">
        <stringProp name="LoopController.loops">-1</stringProp>
      </elementProp>
      <stringProp name="ThreadGroup.duration">120</stringProp>
    </ThreadGroup>
    <CSVDataSet testname="windows" enabled="true">
      <stringProp name="filename">C:\data\users.csv</stringProp>
    </CSVDataSet>
    <CSVDataSet testname="switched off" enabled="false">
      <stringProp name="filename">/absolute/but/disabled.csv</stringProp>
    </CSVDataSet>
    <CSVDataSet testname="no filename" enabled="true"/>
  </hashTree>
</jmeterTestPlan>`

	rep, err := jmx.Inspect(strings.NewReader(plan))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}

	tg := rep.ThreadGroups[0]
	// A property JMeter will resolve at run time is not a number; reporting 0
	// beats refusing the import.
	if tg.Threads != 0 || tg.RampUpSeconds != 0 {
		t.Errorf("non-numeric properties should read as 0, got %+v", tg)
	}
	// The nested LoopController's own property must not leak into the group's.
	if tg.DurationSeconds != 120 {
		t.Errorf("DurationSeconds = %d, want 120 (nested elementProp must not confuse it)", tg.DurationSeconds)
	}

	// Only the enabled, named CSV set counts.
	if len(rep.DataFiles) != 1 || rep.DataFiles[0] != `C:\data\users.csv` {
		t.Errorf("DataFiles = %v, want just the enabled Windows path", rep.DataFiles)
	}
	var unreachable int
	for _, f := range rep.Findings {
		if f.Kind == jmx.FindingUnreachablePath {
			unreachable++
		}
	}
	if unreachable != 1 {
		t.Errorf("unreachable findings = %d, want 1 (the Windows path; the disabled set is ignored)", unreachable)
	}
}
