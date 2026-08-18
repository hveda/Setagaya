package campaign

import "sort"

// ServiceSignal is the minimal per-service go/no-go signal a campaign
// comparison classifies. Deliberately smaller than campaignapp's
// ServiceVerdict (which cannot be imported here: domain packages do not
// depend on the application layer) -- the classifier only ever needs the
// binary result, never the report detail behind it.
type ServiceSignal struct {
	ProjectID int64
	Go        bool
}

// ComparisonStatus classifies one project-service's change between a
// campaign and its baseline.
type ComparisonStatus string

const (
	// ComparisonImproved marks a project that was no-go in the baseline and
	// is go now.
	ComparisonImproved ComparisonStatus = "improved"
	// ComparisonRegressed marks a project that was go in the baseline and is
	// no-go now.
	ComparisonRegressed ComparisonStatus = "regressed"
	// ComparisonNewlyAtRisk marks a project that is no-go now and did not
	// participate in the baseline at all -- a fresh concern, not a
	// regression.
	ComparisonNewlyAtRisk ComparisonStatus = "newly_at_risk"
	// ComparisonStillAtRisk marks a project that is no-go in both -- an
	// ongoing concern, not new.
	ComparisonStillAtRisk ComparisonStatus = "still_at_risk"
	// ComparisonSteady marks a project that is go in both -- no change to
	// report.
	ComparisonSteady ComparisonStatus = "steady"
	// ComparisonNew marks a project that participates now but not in the
	// baseline, and is currently go.
	ComparisonNew ComparisonStatus = "new"
	// ComparisonDropped marks a project that participated in the baseline
	// but not now, regardless of its baseline go/no-go.
	ComparisonDropped ComparisonStatus = "dropped"
)

// ServiceComparison is one project-service's classification against the
// baseline campaign. HasCurrent is false only for ComparisonDropped, in
// which case Go is meaningless (the zero value); HasBaseline is false only
// for ComparisonNew and ComparisonNewlyAtRisk, in which case BaselineGo is
// meaningless.
type ServiceComparison struct {
	ProjectID   int64
	Status      ComparisonStatus
	HasCurrent  bool
	Go          bool
	HasBaseline bool
	BaselineGo  bool
}

// Compare classifies every project that participates in current and/or
// baseline, matched by ProjectID, returned ordered by ProjectID for a stable,
// deterministic result.
func Compare(current, baseline []ServiceSignal) []ServiceComparison {
	curr := make(map[int64]bool, len(current))
	for _, s := range current {
		curr[s.ProjectID] = s.Go
	}
	base := make(map[int64]bool, len(baseline))
	for _, s := range baseline {
		base[s.ProjectID] = s.Go
	}

	ids := make(map[int64]struct{}, len(curr)+len(base))
	for id := range curr {
		ids[id] = struct{}{}
	}
	for id := range base {
		ids[id] = struct{}{}
	}
	sorted := make([]int64, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	out := make([]ServiceComparison, 0, len(sorted))
	for _, id := range sorted {
		currGo, hasCurr := curr[id]
		baseGo, hasBase := base[id]
		out = append(out, ServiceComparison{
			ProjectID: id, HasCurrent: hasCurr, Go: currGo, HasBaseline: hasBase, BaselineGo: baseGo,
			Status: classify(hasCurr, currGo, hasBase, baseGo),
		})
	}
	return out
}

func classify(hasCurr, currGo, hasBase, baseGo bool) ComparisonStatus {
	switch {
	case !hasCurr:
		return ComparisonDropped
	case !hasBase:
		if currGo {
			return ComparisonNew
		}
		return ComparisonNewlyAtRisk
	case currGo && !baseGo:
		return ComparisonImproved
	case !currGo && baseGo:
		return ComparisonRegressed
	case !currGo && !baseGo:
		return ComparisonStillAtRisk
	default: // currGo && baseGo
		return ComparisonSteady
	}
}
