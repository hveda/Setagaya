package fake_test

import (
	"testing"

	"github.com/heridotlife/Setagaya/internal/domain/engine"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
)

func TestFakeMetricsSink_RecordsAndDeletes(t *testing.T) {
	t.Parallel()
	s := fake.NewMetricsSink()

	s.Record(engine.Metric{Label: "a"})
	s.Record(engine.Metric{Label: "b"})
	if got := s.Recorded(); len(got) != 2 || got[0].Label != "a" {
		t.Fatalf("recorded = %+v", got)
	}

	s.DeleteExecution(7)
	if got := s.Deleted(); len(got) != 1 || got[0] != 7 {
		t.Fatalf("deleted = %+v", got)
	}
}
