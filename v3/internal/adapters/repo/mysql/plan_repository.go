package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	driver "github.com/go-sql-driver/mysql"

	"github.com/hveda/Setagaya/v3/internal/domain/plan"
	"github.com/hveda/Setagaya/v3/internal/ports"
)

const planColumns = "id, name, project_id, tenant_id, created_by, updated_by, created_time"

const mysqlDupEntry = 1062

// CreatePlan inserts p and returns its auto-assigned ID.
func (r *Repository) CreatePlan(ctx context.Context, p plan.Plan) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO plan (name, project_id, tenant_id, created_by, updated_by) VALUES (?, ?, ?, ?, ?)",
		p.Name, p.ProjectID, nullInt64(p.TenantID), nullString(p.CreatedBy), nullString(p.UpdatedBy),
	)
	if err != nil {
		return 0, fmt.Errorf("mysql: create plan: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("mysql: create plan last id: %w", err)
	}
	return id, nil
}

// GetPlan returns the plan with id, or ports.ErrNotFound.
func (r *Repository) GetPlan(ctx context.Context, id int64) (plan.Plan, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+planColumns+" FROM plan WHERE id = ?", id)
	p, err := scanPlan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return plan.Plan{}, ports.ErrNotFound
	}
	if err != nil {
		return plan.Plan{}, fmt.Errorf("mysql: get plan: %w", err)
	}
	return p, nil
}

// ListPlansByProject returns all plans belonging to projectID.
func (r *Repository) ListPlansByProject(ctx context.Context, projectID int64) ([]plan.Plan, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+planColumns+" FROM plan WHERE project_id = ?", projectID)
	if err != nil {
		return nil, fmt.Errorf("mysql: list plans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []plan.Plan{}
	for rows.Next() {
		p, scanErr := scanPlan(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("mysql: scan plan: %w", scanErr)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate plans: %w", err)
	}
	return out, nil
}

// DeletePlan removes the plan with id, or returns ports.ErrNotFound.
func (r *Repository) DeletePlan(ctx context.Context, id int64) error {
	return execDelete(ctx, r.db, "DELETE FROM plan WHERE id = ?", id)
}

// AddPlanFile records a file for the plan. isTest selects the single JMX slot
// (plan_test_file) vs a data file (plan_data). Returns ports.ErrFileExists on
// duplicate.
func (r *Repository) AddPlanFile(ctx context.Context, planID int64, filename string, isTest bool) error {
	table := "plan_data"
	if isTest {
		table = "plan_test_file"
	}
	// #nosec G201 -- table is a fixed internal constant, not user input.
	_, err := r.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (plan_id, filename) VALUES (?, ?)", table), planID, filename)
	if isDuplicateKey(err) {
		return ports.ErrFileExists
	}
	if err != nil {
		return fmt.Errorf("mysql: add plan file: %w", err)
	}
	return nil
}

// PlanFilesFor returns the plan's recorded files.
func (r *Repository) PlanFilesFor(ctx context.Context, planID int64) (ports.PlanFiles, error) {
	var pf ports.PlanFiles

	var testFile sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT filename FROM plan_test_file WHERE plan_id = ?", planID).Scan(&testFile)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ports.PlanFiles{}, fmt.Errorf("mysql: plan test file: %w", err)
	}
	pf.TestFile = testFile.String

	rows, err := r.db.QueryContext(ctx, "SELECT filename FROM plan_data WHERE plan_id = ?", planID)
	if err != nil {
		return ports.PlanFiles{}, fmt.Errorf("mysql: plan data files: %w", err)
	}
	defer func() { _ = rows.Close() }()
	pf.Data = []string{}
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			return ports.PlanFiles{}, fmt.Errorf("mysql: scan plan data: %w", scanErr)
		}
		pf.Data = append(pf.Data, name)
	}
	if err := rows.Err(); err != nil {
		return ports.PlanFiles{}, fmt.Errorf("mysql: iterate plan data: %w", err)
	}
	return pf, nil
}

// DeletePlanFile removes a file record for the plan, or ports.ErrNotFound.
func (r *Repository) DeletePlanFile(ctx context.Context, planID int64, filename string, isTest bool) error {
	table := "plan_data"
	if isTest {
		table = "plan_test_file"
	}
	// #nosec G201 -- table is a fixed internal constant, not user input.
	return execDelete(ctx, r.db, fmt.Sprintf("DELETE FROM %s WHERE plan_id = ? AND filename = ?", table), planID, filename)
}

// PlanInUse reports whether the plan is referenced by any collection's
// execution configuration.
func (r *Repository) PlanInUse(ctx context.Context, planID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM collection_plan WHERE plan_id = ?)", planID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("mysql: plan in use: %w", err)
	}
	return exists, nil
}

func scanPlan(s rowScanner) (plan.Plan, error) {
	var (
		p         plan.Plan
		tenantID  sql.NullInt64
		createdBy sql.NullString
		updatedBy sql.NullString
	)
	if err := s.Scan(&p.ID, &p.Name, &p.ProjectID, &tenantID, &createdBy, &updatedBy, &p.CreatedTime); err != nil {
		return plan.Plan{}, err
	}
	p.CreatedBy = createdBy.String
	p.UpdatedBy = updatedBy.String
	if tenantID.Valid {
		p.TenantID = &tenantID.Int64
	}
	return p, nil
}

func isDuplicateKey(err error) bool {
	var mysqlErr *driver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == mysqlDupEntry
}
