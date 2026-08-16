// Package jmx inspects a JMeter test plan so a Shibuya user can import one and
// be told what will and will not survive the move to Honryu.
//
// Nothing is converted. Taurus consumes a .jmx directly as a scenario's script,
// so an imported plan becomes a native, JMeter-pinned scenario unchanged; the
// work here is reporting what Honryu will do differently, because the dangerous
// import is the one that runs but not as its author intended.
//
// Pure domain: parsing only, no file or network access.
package jmx

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Inspection errors. Callers compare with errors.Is.
var (
	ErrMalformed = errors.New("jmx: file is not well-formed XML")
	ErrNotJMX    = errors.New("jmx: file is not a JMeter test plan")
	// ErrNoTestPlan is a structurally valid .jmx that carries no TestPlan --
	// a WorkBench or recording-proxy file, for instance. It parses but cannot
	// be run, so importing one would produce a scenario that fails only when a
	// pod starts.
	ErrNoTestPlan = errors.New("jmx: file contains no TestPlan element")
)

// Finding kinds. They are stable strings because they cross the API boundary.
const (
	// FindingLoadOverridden marks load settings baked into the plan that
	// Honryu's load profile supersedes.
	FindingLoadOverridden = "load-overridden"
	// FindingUnreachablePath marks a data file the engine pod will not find.
	FindingUnreachablePath = "unreachable-path"
	// FindingListenerIgnored marks a listener whose output Honryu does not read.
	FindingListenerIgnored = "listener-ignored"
	// FindingExternalReporting marks a listener that ships results off-platform.
	FindingExternalReporting = "external-reporting"
)

// Finding is something the importer must tell the user rather than silently
// accept or drop.
type Finding struct {
	Kind    string `json:"kind"`
	Element string `json:"element"`
	Detail  string `json:"detail"`
}

// ThreadGroup is a plan's own load settings, recorded for the record: Honryu
// drives concurrency and duration from the execution's load profile instead.
type ThreadGroup struct {
	Name            string `json:"name"`
	Threads         int    `json:"threads"`
	RampUpSeconds   int    `json:"ramp_up_seconds"`
	DurationSeconds int    `json:"duration_seconds"`
}

// Report is what an import tells the user about their plan.
type Report struct {
	TestPlanName string        `json:"test_plan_name"`
	ThreadGroups []ThreadGroup `json:"thread_groups,omitempty"`
	// DataFiles are the CSV files the plan expects to read. They must be
	// uploaded to the scenario or the run fails when the pod cannot find them.
	DataFiles []string  `json:"data_files,omitempty"`
	Findings  []Finding `json:"findings,omitempty"`
}

// Inspect parses a JMeter test plan.
func Inspect(r io.Reader) (Report, error) {
	dec := xml.NewDecoder(r)
	var (
		rep          Report
		seenRoot     bool
		seenTestPlan bool
	)

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Report{}, fmt.Errorf("%w: %s", ErrMalformed, err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if !seenRoot {
			if start.Name.Local != "jmeterTestPlan" {
				return Report{}, fmt.Errorf("%w: root element is %q", ErrNotJMX, start.Name.Local)
			}
			seenRoot = true
			continue
		}

		switch start.Name.Local {
		case "TestPlan":
			seenTestPlan = true
			rep.TestPlanName = attr(start, "testname")
		case "ThreadGroup", "SetupThreadGroup", "PostThreadGroup":
			tg, err := readThreadGroup(dec, start)
			if err != nil {
				return Report{}, err
			}
			rep.ThreadGroups = append(rep.ThreadGroups, tg)
			rep.Findings = append(rep.Findings, Finding{
				Kind:    FindingLoadOverridden,
				Element: start.Name.Local,
				Detail: fmt.Sprintf(
					"thread group %q sets %d threads, %ds ramp-up, %ds duration; Honryu runs it at the execution's load profile instead",
					tg.Name, tg.Threads, tg.RampUpSeconds, tg.DurationSeconds),
			})
		case "CSVDataSet":
			if !enabled(start) {
				continue
			}
			props, err := readProps(dec, start)
			if err != nil {
				return Report{}, err
			}
			name := props["filename"]
			if name == "" {
				continue
			}
			rep.DataFiles = append(rep.DataFiles, name)
			if !podReachable(name) {
				rep.Findings = append(rep.Findings, Finding{
					Kind:    FindingUnreachablePath,
					Element: "CSVDataSet",
					Detail: fmt.Sprintf(
						"%q reads %s, which an engine pod cannot resolve; upload it to the scenario and reference it by name",
						attr(start, "testname"), name),
				})
			}
		case "ResultCollector":
			if !enabled(start) {
				continue
			}
			rep.Findings = append(rep.Findings, Finding{
				Kind:    FindingListenerIgnored,
				Element: "ResultCollector",
				Detail: fmt.Sprintf(
					"listener %q writes its own results; Honryu reports from the engine's own output and will not read them",
					attr(start, "testname")),
			})
		case "BackendListener":
			if !enabled(start) {
				continue
			}
			rep.Findings = append(rep.Findings, Finding{
				Kind:    FindingExternalReporting,
				Element: "BackendListener",
				Detail: fmt.Sprintf(
					"listener %q ships results to another system; it will still run and send data off-platform",
					attr(start, "testname")),
			})
		}
	}

	if !seenRoot {
		return Report{}, fmt.Errorf("%w: no elements found", ErrMalformed)
	}
	if !seenTestPlan {
		return Report{}, ErrNoTestPlan
	}
	return rep, nil
}

// readThreadGroup collects a thread group's own load settings.
func readThreadGroup(dec *xml.Decoder, start xml.StartElement) (ThreadGroup, error) {
	props, err := readProps(dec, start)
	if err != nil {
		return ThreadGroup{}, err
	}
	return ThreadGroup{
		Name:            attr(start, "testname"),
		Threads:         atoi(props["ThreadGroup.num_threads"]),
		RampUpSeconds:   atoi(props["ThreadGroup.ramp_time"]),
		DurationSeconds: atoi(props["ThreadGroup.duration"]),
	}, nil
}

// readProps consumes an element, returning its stringProp values by name.
func readProps(dec *xml.Decoder, start xml.StartElement) (map[string]string, error) {
	props := map[string]string{}
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return props, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrMalformed, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "stringProp" || t.Name.Local == "intProp" {
				name := attr(t, "name")
				var val string
				if err := dec.DecodeElement(&val, &t); err != nil {
					return nil, fmt.Errorf("%w: %s", ErrMalformed, err)
				}
				depth--
				if _, exists := props[name]; !exists {
					props[name] = strings.TrimSpace(val)
				}
			}
		case xml.EndElement:
			if depth == 0 && t.Name.Local == start.Name.Local {
				return props, nil
			}
			depth--
		}
	}
}

// podReachable reports whether a data-file reference can resolve inside an
// engine pod, where only files uploaded to the scenario are mounted.
func podReachable(path string) bool {
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	// A drive-letter path (C:\data\users.csv) is equally unreachable.
	if len(path) > 1 && path[1] == ':' {
		return false
	}
	return true
}

func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// enabled reports whether an element participates in the run. JMeter writes
// enabled="false" for elements the author switched off, and reporting those
// would be noise.
func enabled(e xml.StartElement) bool {
	return attr(e, "enabled") != "false"
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
