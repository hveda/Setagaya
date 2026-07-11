package fake_test

import (
	"testing"

	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
	"github.com/heridotlife/Setagaya/internal/ports/objectstoretest"
	"github.com/heridotlife/Setagaya/internal/ports/repositorytest"
)

func TestFakeStore_PlanContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunPlanRepositoryContract(t, func(_ *testing.T) repositorytest.Repository {
		return fake.NewStore()
	})
}

func TestFakeStore_CollectionContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunCollectionRepositoryContract(t, func(_ *testing.T) repositorytest.Repository {
		return fake.NewStore()
	})
}

func TestFakeObjectStore_Contract(t *testing.T) {
	t.Parallel()
	objectstoretest.RunObjectStoreContract(t, func(_ *testing.T) ports.ObjectStore {
		return fake.NewObjectStore()
	})
}

func TestFakeStore_TenantContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunTenantRepositoryContract(t, func(_ *testing.T) ports.TenantRepository {
		return fake.NewStore()
	})
}

func TestFakeStore_RoleAssignmentContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunRoleAssignmentRepositoryContract(t, func(_ *testing.T) ports.RoleAssignmentRepository {
		return fake.NewStore()
	})
}
