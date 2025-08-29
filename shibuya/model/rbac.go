package model

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rakutentech/shibuya/shibuya/config"
)

// Role represents a user role in the RBAC system
type Role struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedTime time.Time `json:"created_time"`
	UpdatedTime time.Time `json:"updated_time"`
	Permissions []Permission `json:"permissions,omitempty"`
}

// Permission represents a granular permission in the RBAC system
type Permission struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Resource    string    `json:"resource"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	CreatedTime time.Time `json:"created_time"`
}

// User represents a user in the system
type User struct {
	ID            int64     `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	FullName      string    `json:"full_name"`
	PrimaryRoleID *int64    `json:"primary_role_id"`
	PrimaryRole   *Role     `json:"primary_role,omitempty"`
	IsActive      bool      `json:"is_active"`
	LastLogin     *time.Time `json:"last_login"`
	CreatedTime   time.Time `json:"created_time"`
	UpdatedTime   time.Time `json:"updated_time"`
	Roles         []Role    `json:"roles,omitempty"`
}

// UserRole represents the mapping between users and roles
type UserRole struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	RoleID      int64      `json:"role_id"`
	Role        *Role      `json:"role,omitempty"`
	GrantedBy   string     `json:"granted_by"`
	GrantedTime time.Time  `json:"granted_time"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// CreateRole creates a new role
func CreateRole(name, description string) (*Role, error) {
	db := config.SC.DBC
	q, err := db.Prepare("INSERT INTO roles (name, description) VALUES (?, ?)")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	result, err := q.Exec(name, description)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetRole(id)
}

// GetRole retrieves a role by ID
func GetRole(id int64) (*Role, error) {
	db := config.SC.DBC
	q, err := db.Prepare("SELECT id, name, description, created_time, updated_time FROM roles WHERE id = ?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	role := &Role{}
	err = q.QueryRow(id).Scan(&role.ID, &role.Name, &role.Description, &role.CreatedTime, &role.UpdatedTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &DBError{Err: err, Message: "role not found"}
		}
		return nil, err
	}

	return role, nil
}

// GetRoleByName retrieves a role by name
func GetRoleByName(name string) (*Role, error) {
	db := config.SC.DBC
	q, err := db.Prepare("SELECT id, name, description, created_time, updated_time FROM roles WHERE name = ?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	role := &Role{}
	err = q.QueryRow(name).Scan(&role.ID, &role.Name, &role.Description, &role.CreatedTime, &role.UpdatedTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &DBError{Err: err, Message: "role not found"}
		}
		return nil, err
	}

	return role, nil
}

// GetAllRoles retrieves all roles
func GetAllRoles() ([]Role, error) {
	db := config.SC.DBC
	q, err := db.Prepare("SELECT id, name, description, created_time, updated_time FROM roles ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	rows, err := q.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		role := Role{}
		err = rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedTime, &role.UpdatedTime)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	return roles, rows.Err()
}

// UpdateRole updates a role
func (r *Role) Update() error {
	db := config.SC.DBC
	q, err := db.Prepare("UPDATE roles SET name = ?, description = ?, updated_time = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(r.Name, r.Description, r.ID)
	return err
}

// Delete deletes a role
func (r *Role) Delete() error {
	db := config.SC.DBC
	q, err := db.Prepare("DELETE FROM roles WHERE id = ?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(r.ID)
	return err
}

// GetPermissions retrieves all permissions for a role
func (r *Role) GetPermissions() ([]Permission, error) {
	db := config.SC.DBC
	q, err := db.Prepare(`
		SELECT p.id, p.name, p.resource, p.action, p.description, p.created_time 
		FROM permissions p 
		JOIN role_permissions rp ON p.id = rp.permission_id 
		WHERE rp.role_id = ?
		ORDER BY p.resource, p.action
	`)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	rows, err := q.Query(r.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		perm := Permission{}
		err = rows.Scan(&perm.ID, &perm.Name, &perm.Resource, &perm.Action, &perm.Description, &perm.CreatedTime)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}

	return permissions, rows.Err()
}

// CreatePermission creates a new permission
func CreatePermission(name, resource, action, description string) (*Permission, error) {
	db := config.SC.DBC
	q, err := db.Prepare("INSERT INTO permissions (name, resource, action, description) VALUES (?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	result, err := q.Exec(name, resource, action, description)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetPermission(id)
}

// GetPermission retrieves a permission by ID
func GetPermission(id int64) (*Permission, error) {
	db := config.SC.DBC
	q, err := db.Prepare("SELECT id, name, resource, action, description, created_time FROM permissions WHERE id = ?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	perm := &Permission{}
	err = q.QueryRow(id).Scan(&perm.ID, &perm.Name, &perm.Resource, &perm.Action, &perm.Description, &perm.CreatedTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &DBError{Err: err, Message: "permission not found"}
		}
		return nil, err
	}

	return perm, nil
}

// GetPermissionByName retrieves a permission by name
func GetPermissionByName(name string) (*Permission, error) {
	db := config.SC.DBC
	q, err := db.Prepare("SELECT id, name, resource, action, description, created_time FROM permissions WHERE name = ?")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	perm := &Permission{}
	err = q.QueryRow(name).Scan(&perm.ID, &perm.Name, &perm.Resource, &perm.Action, &perm.Description, &perm.CreatedTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &DBError{Err: err, Message: "permission not found"}
		}
		return nil, err
	}

	return perm, nil
}

// GetAllPermissions retrieves all permissions
func GetAllPermissions() ([]Permission, error) {
	db := config.SC.DBC
	q, err := db.Prepare("SELECT id, name, resource, action, description, created_time FROM permissions ORDER BY resource, action")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	rows, err := q.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		perm := Permission{}
		err = rows.Scan(&perm.ID, &perm.Name, &perm.Resource, &perm.Action, &perm.Description, &perm.CreatedTime)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}

	return permissions, rows.Err()
}

// CreateUser creates a new user
func CreateUser(username, email, fullName string, primaryRoleID *int64) (*User, error) {
	db := config.SC.DBC
	q, err := db.Prepare("INSERT INTO users (username, email, full_name, primary_role_id) VALUES (?, ?, ?, ?)")
	if err != nil {
		return nil, err
	}
	defer q.Close()

	result, err := q.Exec(username, email, fullName, primaryRoleID)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetUser(id)
}

// GetUser retrieves a user by ID
func GetUser(id int64) (*User, error) {
	db := config.SC.DBC
	q, err := db.Prepare(`
		SELECT u.id, u.username, u.email, u.full_name, u.primary_role_id, u.is_active, 
		       u.last_login, u.created_time, u.updated_time
		FROM users u 
		WHERE u.id = ?
	`)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	user := &User{}
	var lastLogin sql.NullTime
	var primaryRoleID sql.NullInt64

	err = q.QueryRow(id).Scan(
		&user.ID, &user.Username, &user.Email, &user.FullName, &primaryRoleID, 
		&user.IsActive, &lastLogin, &user.CreatedTime, &user.UpdatedTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &DBError{Err: err, Message: "user not found"}
		}
		return nil, err
	}

	if primaryRoleID.Valid {
		user.PrimaryRoleID = &primaryRoleID.Int64
	}
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return user, nil
}

// GetUserByUsername retrieves a user by username
func GetUserByUsername(username string) (*User, error) {
	db := config.SC.DBC
	q, err := db.Prepare(`
		SELECT u.id, u.username, u.email, u.full_name, u.primary_role_id, u.is_active, 
		       u.last_login, u.created_time, u.updated_time
		FROM users u 
		WHERE u.username = ?
	`)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	user := &User{}
	var lastLogin sql.NullTime
	var primaryRoleID sql.NullInt64

	err = q.QueryRow(username).Scan(
		&user.ID, &user.Username, &user.Email, &user.FullName, &primaryRoleID, 
		&user.IsActive, &lastLogin, &user.CreatedTime, &user.UpdatedTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &DBError{Err: err, Message: "user not found"}
		}
		return nil, err
	}

	if primaryRoleID.Valid {
		user.PrimaryRoleID = &primaryRoleID.Int64
	}
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return user, nil
}

// GetAllUsers retrieves all users
func GetAllUsers() ([]User, error) {
	db := config.SC.DBC
	q, err := db.Prepare(`
		SELECT u.id, u.username, u.email, u.full_name, u.primary_role_id, u.is_active, 
		       u.last_login, u.created_time, u.updated_time
		FROM users u 
		ORDER BY u.username
	`)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	rows, err := q.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		user := User{}
		var lastLogin sql.NullTime
		var primaryRoleID sql.NullInt64

		err = rows.Scan(
			&user.ID, &user.Username, &user.Email, &user.FullName, &primaryRoleID, 
			&user.IsActive, &lastLogin, &user.CreatedTime, &user.UpdatedTime,
		)
		if err != nil {
			return nil, err
		}

		if primaryRoleID.Valid {
			user.PrimaryRoleID = &primaryRoleID.Int64
		}
		if lastLogin.Valid {
			user.LastLogin = &lastLogin.Time
		}

		users = append(users, user)
	}

	return users, rows.Err()
}

// GetUserRoles retrieves all roles for a user
func GetUserRoles(username string) ([]Role, error) {
	db := config.SC.DBC
	q, err := db.Prepare(`
		SELECT r.id, r.name, r.description, r.created_time, r.updated_time
		FROM roles r
		JOIN user_roles ur ON r.id = ur.role_id
		WHERE ur.username = ? AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
		ORDER BY r.name
	`)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	rows, err := q.Query(username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		role := Role{}
		err = rows.Scan(&role.ID, &role.Name, &role.Description, &role.CreatedTime, &role.UpdatedTime)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	return roles, rows.Err()
}

// GetUserPermissions retrieves all permissions for a user across all their roles
func GetUserPermissions(username string) ([]Permission, error) {
	db := config.SC.DBC
	q, err := db.Prepare(`
		SELECT DISTINCT p.id, p.name, p.resource, p.action, p.description, p.created_time
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		JOIN user_roles ur ON rp.role_id = ur.role_id
		WHERE ur.username = ? AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
		ORDER BY p.resource, p.action
	`)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	rows, err := q.Query(username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		perm := Permission{}
		err = rows.Scan(&perm.ID, &perm.Name, &perm.Resource, &perm.Action, &perm.Description, &perm.CreatedTime)
		if err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}

	return permissions, rows.Err()
}

// AssignRoleToUser assigns a role to a user
func AssignRoleToUser(username string, roleID int64, grantedBy string, expiresAt *time.Time) error {
	db := config.SC.DBC
	q, err := db.Prepare("INSERT INTO user_roles (username, role_id, granted_by, expires_at) VALUES (?, ?, ?, ?) ON DUPLICATE KEY UPDATE granted_by = VALUES(granted_by), expires_at = VALUES(expires_at)")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(username, roleID, grantedBy, expiresAt)
	return err
}

// RemoveRoleFromUser removes a role from a user
func RemoveRoleFromUser(username string, roleID int64) error {
	db := config.SC.DBC
	q, err := db.Prepare("DELETE FROM user_roles WHERE username = ? AND role_id = ?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(username, roleID)
	return err
}

// HasPermission checks if a user has a specific permission
func HasPermission(username, permissionName string) (bool, error) {
	db := config.SC.DBC
	q, err := db.Prepare(`
		SELECT COUNT(*)
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		JOIN user_roles ur ON rp.role_id = ur.role_id
		WHERE ur.username = ? AND p.name = ? AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
	`)
	if err != nil {
		return false, err
	}
	defer q.Close()

	var count int
	err = q.QueryRow(username, permissionName).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// HasResourcePermission checks if a user has permission for a specific resource and action
func HasResourcePermission(username, resource, action string) (bool, error) {
	db := config.SC.DBC
	q, err := db.Prepare(`
		SELECT COUNT(*)
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		JOIN user_roles ur ON rp.role_id = ur.role_id
		WHERE ur.username = ? AND p.resource = ? AND p.action = ? AND (ur.expires_at IS NULL OR ur.expires_at > NOW())
	`)
	if err != nil {
		return false, err
	}
	defer q.Close()

	var count int
	err = q.QueryRow(username, resource, action).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// UpdateUser updates user information
func (u *User) Update() error {
	db := config.SC.DBC
	q, err := db.Prepare("UPDATE users SET email = ?, full_name = ?, primary_role_id = ?, is_active = ?, updated_time = CURRENT_TIMESTAMP WHERE id = ?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(u.Email, u.FullName, u.PrimaryRoleID, u.IsActive, u.ID)
	return err
}

// UpdateLastLogin updates the user's last login time
func (u *User) UpdateLastLogin() error {
	db := config.SC.DBC
	q, err := db.Prepare("UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE username = ?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(u.Username)
	return err
}

// Delete deletes a user
func (u *User) Delete() error {
	db := config.SC.DBC
	q, err := db.Prepare("DELETE FROM users WHERE id = ?")
	if err != nil {
		return err
	}
	defer q.Close()

	_, err = q.Exec(u.ID)
	return err
}

// GetOrCreateUser retrieves a user by username or creates a new one if it doesn't exist
func GetOrCreateUser(username, email, fullName string) (*User, error) {
	user, err := GetUserByUsername(username)
	if err != nil {
		if dbErr, ok := err.(*DBError); ok && strings.Contains(dbErr.Message, "user not found") {
			// User doesn't exist, create a new one with loadtest_user role as default
			loadtestUserRole, err := GetRoleByName("loadtest_user")
			if err != nil {
				return nil, fmt.Errorf("failed to get default role: %v", err)
			}
			
			user, err = CreateUser(username, email, fullName, &loadtestUserRole.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to create user: %v", err)
			}
			
			// Assign the default role
			err = AssignRoleToUser(username, loadtestUserRole.ID, "system", nil)
			if err != nil {
				return nil, fmt.Errorf("failed to assign default role: %v", err)
			}
		} else {
			return nil, err
		}
	}
	
	return user, nil
}