package metrics

// Interval is one second of measurement from one engine pod, as bzt's
// aggregator reports it.
//
// It carries the response-time buckets rather than percentiles, because
// percentiles cannot be combined across pods afterwards. This is the payload a
// sidecar pushes to the control plane, so its json tags are a wire contract:
// the Python reporter inside the engine pod writes exactly this shape.
type Interval struct {
	// Timestamp is the second this interval covers, in Unix seconds.
	Timestamp int64 `json:"ts"`
	// Label is the request this measures. Honryu assigns labels when it compiles
	// a config, but engines do not all honour them -- JMeter echoes the
	// configured label while apiritif and k6 report the URL -- so a label seen
	// here may still need mapping back to the one Honryu chose.
	Label string `json:"label"`
	// Concurrency is the virtual users active in this pod during the interval.
	Concurrency int `json:"concurrency"`
	// Samples is how many requests completed in the interval.
	Samples int64 `json:"samples"`
	// Succeeded and Failed split Samples by outcome.
	Succeeded int64 `json:"succeeded"`
	Failed    int64 `json:"failed"`
	// Bytes received during the interval.
	Bytes int64 `json:"bytes"`
	// Latency holds the response-time buckets, in seconds.
	Latency Histogram `json:"latency"`
	// ResponseCodes counts each status the target returned. Codes are consistent
	// across engines where messages are not, so they are the reliable key for
	// grouping errors.
	ResponseCodes map[string]int64 `json:"response_codes,omitempty"`
	// Errors are the failures the engine saw, grouped as it grouped them.
	Errors []ErrorGroup `json:"errors,omitempty"`
}

// ErrorGroup is one failure mode within an interval.
type ErrorGroup struct {
	// Message is the engine's own wording. It differs between engines for the
	// same failure, so it is an exemplar for a human, never a grouping key.
	Message string `json:"message"`
	// ResponseCode is the status that produced the failure, where there was one.
	ResponseCode string `json:"response_code,omitempty"`
	Count        int64  `json:"count"`
}

// Batch is what a sidecar pushes in one request: the intervals it has collected,
// stamped with the pod that produced them.
type Batch struct {
	ExecutionID int64 `json:"execution_id"`
	ScenarioID  int64 `json:"scenario_id"`
	RunID       int64 `json:"run_id"`
	// ShardIndex identifies the pod within the execution, matching the shard
	// plan, so a duplicate push can be recognised as such.
	ShardIndex int        `json:"shard_index"`
	Intervals  []Interval `json:"intervals"`
	// Final marks the last batch a pod will send, so the control plane knows the
	// pod finished rather than went silent.
	Final bool `json:"final,omitempty"`
}

// Merge folds an interval's measurements into the receiver, which must already
// describe the same label. Used to combine what several pods reported for the
// same second.
func (i *Interval) Merge(other Interval) {
	i.Concurrency += other.Concurrency
	i.Samples += other.Samples
	i.Succeeded += other.Succeeded
	i.Failed += other.Failed
	i.Bytes += other.Bytes

	if i.Latency == nil {
		i.Latency = Histogram{}
	}
	i.Latency.Merge(other.Latency)

	if len(other.ResponseCodes) > 0 && i.ResponseCodes == nil {
		i.ResponseCodes = map[string]int64{}
	}
	for code, n := range other.ResponseCodes {
		i.ResponseCodes[code] += n
	}

	for _, e := range other.Errors {
		i.mergeError(e)
	}
}

// mergeError combines by response code and message, keeping counts additive so
// an error seen by several pods is reported once with the total.
func (i *Interval) mergeError(e ErrorGroup) {
	for idx := range i.Errors {
		if i.Errors[idx].ResponseCode == e.ResponseCode && i.Errors[idx].Message == e.Message {
			i.Errors[idx].Count += e.Count
			return
		}
	}
	i.Errors = append(i.Errors, e)
}
