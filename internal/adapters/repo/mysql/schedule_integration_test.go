//go:build integration

package mysql_test

import (
	"testing"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
	"github.com/heridotlife/honryu/test/dbtest"
)

func TestMySQLScheduleRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunScheduleRepositoryContract(t, func(t *testing.T) ports.ScheduleRepository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}
