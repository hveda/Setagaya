package fake_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/internal/ports/reportprogresstest"
)

func TestFakeReportProgress_Contract(t *testing.T) {
	reportprogresstest.Run(t, func(*testing.T) ports.ReportProgress {
		return fake.NewReportProgress()
	})
}
