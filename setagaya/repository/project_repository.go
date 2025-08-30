package repository

import (
	"database/sql"
	"fmt"

	"github.com/hveda/setagaya/setagaya/model"
	"github.com/hveda/setagaya/setagaya/service"
)

// projectRepository implements service.ProjectRepository
type projectRepository struct {
	db *sql.DB
}

// NewProjectRepository creates a new project repository
func NewProjectRepository(db *sql.DB) service.ProjectRepository {
	return &projectRepository{
		db: db,
	}
}

// Create creates a new project in the database
func (r *projectRepository) Create(name, owner, info string) (int64, error) {
	query := "INSERT INTO project (name, owner, sid) VALUES (?, ?, ?)"
	
	result, err := r.db.Exec(query, name, owner, info)
	if err != nil {
		return 0, fmt.Errorf("failed to insert project: %w", err)
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get inserted project ID: %w", err)
	}
	
	return id, nil
}

// GetByID retrieves a project by its ID
func (r *projectRepository) GetByID(id int64) (*model.Project, error) {
	query := "SELECT id, name, owner, sid, created_time FROM project WHERE id = ?"
	
	project := &model.Project{}
	err := r.db.QueryRow(query, id).Scan(
		&project.ID,
		&project.Name,
		&project.Owner,
		&project.SID,
		&project.CreatedTime,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("project with ID %d not found", id)
		}
		return nil, fmt.Errorf("failed to query project: %w", err)
	}
	
	return project, nil
}

// GetByOwner retrieves all projects for a specific owner
func (r *projectRepository) GetByOwner(owner string) ([]*model.Project, error) {
	query := "SELECT id, name, owner, sid, created_time FROM project WHERE owner = ? ORDER BY created_time DESC"
	
	rows, err := r.db.Query(query, owner)
	if err != nil {
		return nil, fmt.Errorf("failed to query projects by owner: %w", err)
	}
	defer rows.Close()
	
	var projects []*model.Project
	for rows.Next() {
		project := &model.Project{}
		err := rows.Scan(
			&project.ID,
			&project.Name,
			&project.Owner,
			&project.SID,
			&project.CreatedTime,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project row: %w", err)
		}
		projects = append(projects, project)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during project rows iteration: %w", err)
	}
	
	return projects, nil
}

// Update updates a project's information
func (r *projectRepository) Update(id int64, name, info string) error {
	query := "UPDATE project SET name = ?, sid = ? WHERE id = ?"
	
	result, err := r.db.Exec(query, name, info, id)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	
	if rowsAffected == 0 {
		return fmt.Errorf("project with ID %d not found", id)
	}
	
	return nil
}

// Delete removes a project from the database
func (r *projectRepository) Delete(id int64) error {
	// Start transaction for safe deletion
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()
	
	// First check if project exists
	var exists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM project WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check project existence: %w", err)
	}
	
	if !exists {
		return fmt.Errorf("project with ID %d not found", id)
	}
	
	// Delete related collections first (cascade delete)
	_, err = tx.Exec("DELETE FROM collection WHERE project_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete related collections: %w", err)
	}
	
	// Delete the project
	_, err = tx.Exec("DELETE FROM project WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	
	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit project deletion: %w", err)
	}
	
	return nil
}

// CheckOwnership verifies if a user owns a project
func (r *projectRepository) CheckOwnership(id int64, owner string) (bool, error) {
	query := "SELECT EXISTS(SELECT 1 FROM project WHERE id = ? AND owner = ?)"
	
	var exists bool
	err := r.db.QueryRow(query, id, owner).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check project ownership: %w", err)
	}
	
	return exists, nil
}

// Additional helper methods for enhanced repository functionality

// GetProjectStats returns statistics about projects
func (r *projectRepository) GetProjectStats(ownerFilter string) (*ProjectStats, error) {
	var query string
	var args []interface{}
	
	if ownerFilter != "" {
		query = `
			SELECT 
				COUNT(*) as total_projects,
				COUNT(DISTINCT owner) as unique_owners,
				AVG(TIMESTAMPDIFF(DAY, created_time, NOW())) as avg_age_days
			FROM project 
			WHERE owner = ?`
		args = []interface{}{ownerFilter}
	} else {
		query = `
			SELECT 
				COUNT(*) as total_projects,
				COUNT(DISTINCT owner) as unique_owners,
				AVG(TIMESTAMPDIFF(DAY, created_time, NOW())) as avg_age_days
			FROM project`
	}
	
	stats := &ProjectStats{}
	err := r.db.QueryRow(query, args...).Scan(
		&stats.TotalProjects,
		&stats.UniqueOwners,
		&stats.AvgAgeDays,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to get project stats: %w", err)
	}
	
	return stats, nil
}

// ProjectStats represents project statistics
type ProjectStats struct {
	TotalProjects int     `json:"total_projects"`
	UniqueOwners  int     `json:"unique_owners"`
	AvgAgeDays    float64 `json:"avg_age_days"`
}

// SearchProjects searches for projects by name pattern
func (r *projectRepository) SearchProjects(namePattern, owner string, limit int) ([]*model.Project, error) {
	query := `
		SELECT id, name, owner, sid, created_time 
		FROM project 
		WHERE name LIKE ? AND (? = '' OR owner = ?)
		ORDER BY created_time DESC 
		LIMIT ?`
	
	namePattern = "%" + namePattern + "%"
	rows, err := r.db.Query(query, namePattern, owner, owner, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search projects: %w", err)
	}
	defer rows.Close()
	
	var projects []*model.Project
	for rows.Next() {
		project := &model.Project{}
		err := rows.Scan(
			&project.ID,
			&project.Name,
			&project.Owner,
			&project.SID,
			&project.CreatedTime,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		projects = append(projects, project)
	}
	
	return projects, nil
}