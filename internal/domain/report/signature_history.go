package report

import "sort"

// SignatureGroupBy selects which axis GroupSignatureHistory collapses rows
// onto.
type SignatureGroupBy string

const (
	// GroupByLabel collapses rows onto their label, summing across every
	// response code and side that occurred under it.
	GroupByLabel SignatureGroupBy = "label"
	// GroupByResponseCode collapses rows onto their response code, summing
	// across every label and side that produced it.
	GroupByResponseCode SignatureGroupBy = "response_code"
)

// SignatureBreakdown is one grouped bucket's total, plus every finer-grained
// row that contributed to it. TotalCount is a plain re-sum of the
// contributing rows' counts -- always a safe aggregation. RunCount is
// deliberately not rolled up to the group level: the same run can appear
// under two contributing rows (e.g. one label producing both a 500 and a 404
// in the same run), and summing their RunCounts would double count that run.
// A reader who needs "how many runs saw this label at all" reads Rows.
type SignatureBreakdown struct {
	// Key is the grouped-on value: a label when GroupByLabel, a response
	// code when GroupByResponseCode.
	Key        string
	TotalCount int64
	Rows       []SignatureHistoryRow
}

// GroupSignatureHistory collapses rows -- task 109's cross-run totals, each
// already keyed by (label, response_code, side) -- onto one axis. Ordered
// dominant-first by TotalCount, ties broken by Key.
func GroupSignatureHistory(rows []SignatureHistoryRow, by SignatureGroupBy) []SignatureBreakdown {
	groups := make(map[string]*SignatureBreakdown, len(rows))
	var order []string
	for _, r := range rows {
		key := r.Label
		if by == GroupByResponseCode {
			key = r.ResponseCode
		}
		g, ok := groups[key]
		if !ok {
			g = &SignatureBreakdown{Key: key}
			groups[key] = g
			order = append(order, key)
		}
		g.TotalCount += r.TotalCount
		g.Rows = append(g.Rows, r)
	}

	out := make([]SignatureBreakdown, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalCount != out[j].TotalCount {
			return out[i].TotalCount > out[j].TotalCount
		}
		return out[i].Key < out[j].Key
	})
	return out
}
