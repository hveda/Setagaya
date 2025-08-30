package model

import (
	"github.com/hveda/setagaya/setagaya/config"
)

func setupAndTeardown() error {
	db := config.SC.DBC
	
	// Clean up RBAC tables first due to foreign key constraints
	tables := []string{
		"user_roles",
		"role_permissions", 
		"users",
		"permissions",
		"roles",
		"plan",
		"running_plan",
		"collection",
		"collection_plan",
		"project",
		"collection_run",
		"collection_run_history",
	}
	
	for _, table := range tables {
		q, err := db.Prepare("delete from " + table)
		if err != nil {
			return err
		}
		defer q.Close()
		_, err = q.Exec()
		if err != nil {
			return err
		}
	}
	
	return nil
}
