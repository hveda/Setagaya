package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/hveda/Setagaya/setagaya/config"
)

func TestCreateAndGetProject(t *testing.T) {
	// Skip database tests in test mode (when no real DB connection available)
	if os.Getenv("SETAGAYA_TEST_MODE") == "true" || config.SC.DBC == nil {
		t.Skip("Skipping database test in test mode")
		return
	}

	name := "testplan"
	projectID, err := CreateProject(name, "tech-rwasp", "1111")
	if err != nil {
		t.Fatal(err)
	}
	p, err := GetProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, name, p.Name)
	p.Delete()
	p, err = GetProject(projectID)
	assert.NotNil(t, err)
	assert.Nil(t, p)
}

func TestGetProjectCollections(t *testing.T) {
	// Skip database tests in test mode (when no real DB connection available)
	if os.Getenv("SETAGAYA_TEST_MODE") == "true" || config.SC.DBC == nil {
		t.Skip("Skipping database test in test mode")
		return
	}

	name := "testplan"
	projectID, err := CreateProject(name, "tech-rwasp", "1111")
	if err != nil {
		t.Fatal(err)
	}
	p, err := GetProject(projectID)
	if err != nil {
		t.Fatal(err)
	}
	collection_id, err := CreateCollection("testcollection", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	collections, err := p.GetCollections()
	if err != nil {
		t.Fatal(err)
	}
	for _, cid := range collections {
		assert.Equal(t, collection_id, cid)
	}
}
