"""Honryu's bzt reporter: streams the unified KPI stream to disk for the sidecar.

bzt normalises every engine's results into one aggregated stream -- the same
fields whether JMeter, k6, or Gatling produced them -- and hands it to any module
implementing AggregatorListener. This writes that stream out as JSON lines for
the Honryu sidecar to forward.

Two properties matter more than they look:

* It writes *buckets*, not percentiles. Percentiles from separate pods cannot be
  combined afterwards; the counts they came from can.
* It flushes every line. On SIGTERM -- what Kubernetes sends when it deletes a
  pod -- bzt has no signal handler and dies immediately, writing no final
  report. Anything not already on disk at that moment is lost, so nothing is
  buffered waiting for an orderly shutdown that will not come.

Install by adding to a Taurus config:

    modules:
      honryu:
        class: honryu_kpi.HonryuKPIReporter
        filename: /honryu/kpi/stream.jsonl
    reporting:
      - module: honryu
"""

import json
import os

from bzt.engine import Reporter, Singletone
from bzt.modules.aggregator import AggregatorListener, DataPoint, KPISet, ResultsProvider


class HonryuKPIReporter(Reporter, AggregatorListener, Singletone):
    """Writes bzt's per-second aggregated KPIs as JSON lines."""

    def __init__(self):
        super().__init__()
        self.out = None
        self.path = None

    def prepare(self):
        super().prepare()
        self.path = self.settings.get("filename", "/honryu/kpi/stream.jsonl")
        directory = os.path.dirname(self.path)
        if directory:
            os.makedirs(directory, exist_ok=True)
        # Line buffered: a line is on disk as soon as it is written, so a pod
        # killed mid-run loses only the second in progress.
        self.out = open(self.path, "w", buffering=1)  # noqa: SIM115 -- closed in post_process
        if isinstance(self.engine.aggregator, ResultsProvider):
            self.engine.aggregator.add_listener(self)

    def aggregated_second(self, data):
        """Called once per second with every label's measurements."""
        ts = data[DataPoint.TIMESTAMP]
        for label, kpi in data[DataPoint.CURRENT].items():
            self._write(ts, label, kpi)

    def _write(self, ts, label, kpi):
        latency = kpi[KPISet.RESP_TIMES]
        record = {
            "ts": ts,
            # bzt uses "" for the aggregate across all labels; name it so the
            # control plane never has to guess what an empty label meant.
            "label": label or "__total__",
            "concurrency": kpi[KPISet.CONCURRENCY],
            "samples": kpi[KPISet.SAMPLE_COUNT],
            "succeeded": kpi[KPISet.SUCCESSES],
            "failed": kpi[KPISet.FAILURES],
            "bytes": kpi[KPISet.BYTE_COUNT],
            # Buckets, not percentiles: {response_time_seconds: count}.
            "latency": latency.__json__() if latency else {},
            "response_codes": dict(kpi[KPISet.RESP_CODES]),
            "errors": [
                {
                    "message": err.get("msg"),
                    "response_code": err.get("rc"),
                    "count": err.get("cnt"),
                }
                for err in kpi[KPISet.ERRORS]
            ],
        }
        self.out.write(json.dumps(record) + "\n")

    def post_process(self):
        if self.out:
            self.out.close()
            self.out = None
        super().post_process()
