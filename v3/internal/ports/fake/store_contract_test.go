package fake_test

import (
	"testing"

	"github.com/hveda/Setagaya/v3/internal/ports"
	"github.com/hveda/Setagaya/v3/internal/ports/fake"
	"github.com/hveda/Setagaya/v3/internal/ports/objectstoretest"
	"github.com/hveda/Setagaya/v3/internal/ports/repositorytest"
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
