package model

import (
	"testing"
	"time"

	"github.com/hveda/setagaya/setagaya/config"
	"github.com/stretchr/testify/assert"
)

func TestCreateAndGetRole(t *testing.T) {
	// Cleanup
	defer setupAndTeardown()

	name := "test_role"
	description := "Test role description"

	// Create role
	role, err := CreateRole(name, description)
	assert.NoError(t, err)
	assert.NotNil(t, role)
	assert.Equal(t, name, role.Name)
	assert.Equal(t, description, role.Description)
	assert.Greater(t, role.ID, int64(0))

	// Get role by ID
	retrievedRole, err := GetRole(role.ID)
	assert.NoError(t, err)
	assert.Equal(t, role.ID, retrievedRole.ID)
	assert.Equal(t, role.Name, retrievedRole.Name)
	assert.Equal(t, role.Description, retrievedRole.Description)

	// Get role by name
	roleByName, err := GetRoleByName(name)
	assert.NoError(t, err)
	assert.Equal(t, role.ID, roleByName.ID)
	assert.Equal(t, role.Name, roleByName.Name)

	// Delete role
	err = role.Delete()
	assert.NoError(t, err)

	// Verify deletion
	_, err = GetRole(role.ID)
	assert.Error(t, err)
}

func TestCreateAndGetPermission(t *testing.T) {
	// Cleanup
	defer setupAndTeardown()

	name := "test:permission"
	resource := "test"
	action := "permission"
	description := "Test permission description"

	// Create permission
	permission, err := CreatePermission(name, resource, action, description)
	assert.NoError(t, err)
	assert.NotNil(t, permission)
	assert.Equal(t, name, permission.Name)
	assert.Equal(t, resource, permission.Resource)
	assert.Equal(t, action, permission.Action)
	assert.Equal(t, description, permission.Description)
	assert.Greater(t, permission.ID, int64(0))

	// Get permission by ID
	retrievedPermission, err := GetPermission(permission.ID)
	assert.NoError(t, err)
	assert.Equal(t, permission.ID, retrievedPermission.ID)
	assert.Equal(t, permission.Name, retrievedPermission.Name)

	// Get permission by name
	permissionByName, err := GetPermissionByName(name)
	assert.NoError(t, err)
	assert.Equal(t, permission.ID, permissionByName.ID)
	assert.Equal(t, permission.Name, permissionByName.Name)
}

func TestCreateAndGetUser(t *testing.T) {
	// Cleanup
	defer setupAndTeardown()

	username := "testuser"
	email := "test@example.com"
	fullName := "Test User"

	// Create user
	user, err := CreateUser(username, email, fullName, nil)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, username, user.Username)
	assert.Equal(t, email, user.Email)
	assert.Equal(t, fullName, user.FullName)
	assert.Greater(t, user.ID, int64(0))
	assert.True(t, user.IsActive)

	// Get user by ID
	retrievedUser, err := GetUser(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, retrievedUser.ID)
	assert.Equal(t, user.Username, retrievedUser.Username)

	// Get user by username
	userByUsername, err := GetUserByUsername(username)
	assert.NoError(t, err)
	assert.Equal(t, user.ID, userByUsername.ID)
	assert.Equal(t, user.Username, userByUsername.Username)

	// Update user
	user.Email = "updated@example.com"
	user.FullName = "Updated Test User"
	err = user.Update()
	assert.NoError(t, err)

	// Verify update
	updatedUser, err := GetUser(user.ID)
	assert.NoError(t, err)
	assert.Equal(t, "updated@example.com", updatedUser.Email)
	assert.Equal(t, "Updated Test User", updatedUser.FullName)

	// Delete user
	err = user.Delete()
	assert.NoError(t, err)

	// Verify deletion
	_, err = GetUser(user.ID)
	assert.Error(t, err)
}

func TestUserRoleAssignment(t *testing.T) {
	// Cleanup
	defer setupAndTeardown()

	// Create a role
	role, err := CreateRole("test_role", "Test role")
	assert.NoError(t, err)

	// Create a user
	user, err := CreateUser("testuser", "test@example.com", "Test User", nil)
	assert.NoError(t, err)

	// Assign role to user
	err = AssignRoleToUser(user.Username, role.ID, "admin", nil)
	assert.NoError(t, err)

	// Get user roles
	roles, err := GetUserRoles(user.Username)
	assert.NoError(t, err)
	assert.Len(t, roles, 1)
	assert.Equal(t, role.ID, roles[0].ID)
	assert.Equal(t, role.Name, roles[0].Name)

	// Remove role from user
	err = RemoveRoleFromUser(user.Username, role.ID)
	assert.NoError(t, err)

	// Verify removal
	roles, err = GetUserRoles(user.Username)
	assert.NoError(t, err)
	assert.Len(t, roles, 0)

	// Test role assignment with expiration
	expiresAt := time.Now().Add(24 * time.Hour)
	err = AssignRoleToUser(user.Username, role.ID, "admin", &expiresAt)
	assert.NoError(t, err)

	roles, err = GetUserRoles(user.Username)
	assert.NoError(t, err)
	assert.Len(t, roles, 1)
}

func TestUserPermissions(t *testing.T) {
	// Cleanup
	defer setupAndTeardown()

	// Create permission
	permission, err := CreatePermission("test:read", "test", "read", "Test read permission")
	assert.NoError(t, err)

	// Create role
	role, err := CreateRole("test_role", "Test role")
	assert.NoError(t, err)

	// Create user
	user, err := CreateUser("testuser", "test@example.com", "Test User", nil)
	assert.NoError(t, err)

	// Assign permission to role (this requires direct database manipulation for testing)
	// In a real scenario, you'd have a function to assign permissions to roles
	db := config.SC.DBC
	_, err = db.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?)", role.ID, permission.ID)
	assert.NoError(t, err)

	// Assign role to user
	err = AssignRoleToUser(user.Username, role.ID, "admin", nil)
	assert.NoError(t, err)

	// Check if user has permission
	hasPermission, err := HasPermission(user.Username, "test:read")
	assert.NoError(t, err)
	assert.True(t, hasPermission)

	// Check if user has resource permission
	hasResourcePermission, err := HasResourcePermission(user.Username, "test", "read")
	assert.NoError(t, err)
	assert.True(t, hasResourcePermission)

	// Check for non-existent permission
	hasNonExistentPermission, err := HasPermission(user.Username, "non:existent")
	assert.NoError(t, err)
	assert.False(t, hasNonExistentPermission)

	// Get user permissions
	permissions, err := GetUserPermissions(user.Username)
	assert.NoError(t, err)
	assert.Len(t, permissions, 1)
	assert.Equal(t, permission.ID, permissions[0].ID)
	assert.Equal(t, permission.Name, permissions[0].Name)
}

func TestGetOrCreateUser(t *testing.T) {
	// Cleanup
	defer setupAndTeardown()

	username := "newuser"
	email := "new@example.com"
	fullName := "New User"

	// First call should create the user
	user1, err := GetOrCreateUser(username, email, fullName)
	assert.NoError(t, err)
	assert.NotNil(t, user1)
	assert.Equal(t, username, user1.Username)
	assert.Equal(t, email, user1.Email)
	assert.Equal(t, fullName, user1.FullName)

	// Second call should return the existing user
	user2, err := GetOrCreateUser(username, "different@example.com", "Different Name")
	assert.NoError(t, err)
	assert.NotNil(t, user2)
	assert.Equal(t, user1.ID, user2.ID)
	assert.Equal(t, username, user2.Username)
	// Original email and name should be preserved
	assert.Equal(t, email, user2.Email)
	assert.Equal(t, fullName, user2.FullName)

	// Check that the user has a default role assigned
	roles, err := GetUserRoles(username)
	assert.NoError(t, err)
	assert.Len(t, roles, 1)
	// Should have loadtest_user role as default
	assert.Equal(t, "loadtest_user", roles[0].Name)
}

func TestGetAllFunctions(t *testing.T) {
	// Cleanup
	defer setupAndTeardown()

	// Create test data
	role1, err := CreateRole("role1", "First role")
	assert.NoError(t, err)
	role2, err := CreateRole("role2", "Second role")
	assert.NoError(t, err)

	permission1, err := CreatePermission("perm1", "resource1", "action1", "First permission")
	assert.NoError(t, err)
	permission2, err := CreatePermission("perm2", "resource2", "action2", "Second permission")
	assert.NoError(t, err)

	// Use variables to avoid unused variable errors
	_ = permission1
	_ = permission2

	user1, err := CreateUser("user1", "user1@example.com", "First User", nil)
	assert.NoError(t, err)
	user2, err := CreateUser("user2", "user2@example.com", "Second User", nil)
	assert.NoError(t, err)

	// Test GetAllRoles
	roles, err := GetAllRoles()
	assert.NoError(t, err)
	// Should include our test roles plus any seeded roles
	assert.GreaterOrEqual(t, len(roles), 2)

	// Test GetAllPermissions
	permissions, err := GetAllPermissions()
	assert.NoError(t, err)
	// Should include our test permissions plus any seeded permissions
	assert.GreaterOrEqual(t, len(permissions), 2)

	// Test GetAllUsers
	users, err := GetAllUsers()
	assert.NoError(t, err)
	// Should include our test users
	assert.GreaterOrEqual(t, len(users), 2)

	// Verify some of our data is present
	foundRole1 := false
	for _, role := range roles {
		if role.Name == "role1" {
			foundRole1 = true
			break
		}
	}
	assert.True(t, foundRole1)

	foundPermission1 := false
	for _, permission := range permissions {
		if permission.Name == "perm1" {
			foundPermission1 = true
			break
		}
	}
	assert.True(t, foundPermission1)

	foundUser1 := false
	for _, user := range users {
		if user.Username == "user1" {
			foundUser1 = true
			break
		}
	}
	assert.True(t, foundUser1)

	// Clean up
	role1.Delete()
	role2.Delete()
	user1.Delete()
	user2.Delete()
}