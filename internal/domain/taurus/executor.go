package taurus

import "sort"

// Executor names a load-generation engine as bzt knows it. Honryu does not
// invent engine names: adopting Taurus means the engine set is whatever bzt
// supports, addressed by bzt's own identifiers.
type Executor string

// The executors Honryu models. bzt supports more; these are the ones whose
// behaviour has been established here.
const (
	ExecutorJMeter   Executor = "jmeter"
	ExecutorK6       Executor = "k6"
	ExecutorGatling  Executor = "gatling"
	ExecutorLocust   Executor = "locust"
	ExecutorApiritif Executor = "apiritif"
	ExecutorAB       Executor = "ab"
	ExecutorSiege    Executor = "siege"
)

// declarativeSupport records whether an executor accepts Taurus's declarative
// `requests:` form, or requires an engine-native script.
//
// This is the asymmetry that makes engines only partly interchangeable: bzt
// normalises *results* across engines, but not *inputs*. Established by running
// each engine under bzt (Phase 0) and by reading bzt 1.16's executor modules --
// bzt/modules/k6.py raises "'script' should be present for k6 executor", while
// the others build a script from the request list.
var declarativeSupport = map[Executor]bool{
	ExecutorJMeter:   true,
	ExecutorGatling:  true,
	ExecutorLocust:   true,
	ExecutorApiritif: true,
	ExecutorAB:       true,
	ExecutorSiege:    true,
	ExecutorK6:       false,
}

// Known reports whether Honryu models this executor.
func (e Executor) Known() bool {
	_, ok := declarativeSupport[e]
	return ok
}

// AcceptsDeclarativeRequests reports whether the executor can run a scenario
// expressed as a Taurus request list. Unknown executors report false: the safe
// answer is to require a native script rather than emit a config bzt rejects
// inside a pod.
func (e Executor) AcceptsDeclarativeRequests() bool {
	return declarativeSupport[e]
}

// DeclarativeExecutors lists, in stable order, every executor that can run a
// portable scenario. The returned slice is the caller's to keep.
func DeclarativeExecutors() []Executor {
	out := make([]Executor, 0, len(declarativeSupport))
	for e, declarative := range declarativeSupport {
		if declarative {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
