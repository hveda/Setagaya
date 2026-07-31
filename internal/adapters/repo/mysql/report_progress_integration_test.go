//go:build integration

package mysql_test

import (
	"testing"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/reportprogresstest"
	"github.com/heridotlife/honryu/test/dbtest"
)

func TestMySQLReportProgress_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	reportprogresstest.Run(t, func(t *testing.T) ports.ReportProgress {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}
