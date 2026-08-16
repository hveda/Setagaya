//go:build integration

package mysql_test

import (
	"testing"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
	"github.com/heridotlife/honryu/test/dbtest"
)

func TestMySQLCapacityProfileRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunCapacityProfileRepositoryContract(t, func(t *testing.T) ports.CapacityProfileRepository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}
