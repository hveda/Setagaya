package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/model"
)

// Role management handlers

// rolesGetHandler retrieves all roles
func (s *ShibuyaAPI) rolesGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	roles, err := model.GetAllRoles()
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	// Load permissions for each role
	for i := range roles {
		permissions, err := roles[i].GetPermissions()
		if err != nil {
			s.handleErrors(w, err)
			return
		}
		roles[i].Permissions = permissions
	}

	s.jsonise(w, http.StatusOK, roles)
}

// roleGetHandler retrieves a specific role
func (s *ShibuyaAPI) roleGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	roleIDStr := params.ByName("role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid role ID"))
		return
	}

	role, err := model.GetRole(roleID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	// Load permissions for the role
	permissions, err := role.GetPermissions()
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	role.Permissions = permissions

	s.jsonise(w, http.StatusOK, role)
}

// roleCreateHandler creates a new role
func (s *ShibuyaAPI) roleCreateHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid JSON"))
		return
	}

	if req.Name == "" {
		s.handleErrors(w, makeBadRequestError("Role name is required"))
		return
	}

	role, err := model.CreateRole(req.Name, req.Description)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, role)
}

// roleUpdateHandler updates a role
func (s *ShibuyaAPI) roleUpdateHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	roleIDStr := params.ByName("role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid role ID"))
		return
	}

	role, err := model.GetRole(roleID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid JSON"))
		return
	}

	if req.Name != "" {
		role.Name = req.Name
	}
	role.Description = req.Description

	if err := role.Update(); err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, role)
}

// roleDeleteHandler deletes a role
func (s *ShibuyaAPI) roleDeleteHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	roleIDStr := params.ByName("role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid role ID"))
		return
	}

	role, err := model.GetRole(roleID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	if err := role.Delete(); err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, map[string]string{"message": "Role deleted successfully"})
}

// Permission management handlers

// permissionsGetHandler retrieves all permissions
func (s *ShibuyaAPI) permissionsGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	permissions, err := model.GetAllPermissions()
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, permissions)
}

// permissionGetHandler retrieves a specific permission
func (s *ShibuyaAPI) permissionGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	permissionIDStr := params.ByName("permission_id")
	permissionID, err := strconv.ParseInt(permissionIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid permission ID"))
		return
	}

	permission, err := model.GetPermission(permissionID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, permission)
}

// permissionCreateHandler creates a new permission
func (s *ShibuyaAPI) permissionCreateHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	var req struct {
		Name        string `json:"name"`
		Resource    string `json:"resource"`
		Action      string `json:"action"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid JSON"))
		return
	}

	if req.Name == "" || req.Resource == "" || req.Action == "" {
		s.handleErrors(w, makeBadRequestError("Name, resource, and action are required"))
		return
	}

	permission, err := model.CreatePermission(req.Name, req.Resource, req.Action, req.Description)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, permission)
}

// User management handlers

// usersGetHandler retrieves all users
func (s *ShibuyaAPI) usersGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	users, err := model.GetAllUsers()
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	// Load roles for each user
	for i := range users {
		roles, err := model.GetUserRoles(users[i].Username)
		if err != nil {
			s.handleErrors(w, err)
			return
		}
		users[i].Roles = roles
	}

	s.jsonise(w, http.StatusOK, users)
}

// userGetHandler retrieves a specific user
func (s *ShibuyaAPI) userGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	userIDStr := params.ByName("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid user ID"))
		return
	}

	user, err := model.GetUser(userID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	// Load user roles
	roles, err := model.GetUserRoles(user.Username)
	if err != nil {
		s.handleErrors(w, err)
		return
	}
	user.Roles = roles

	s.jsonise(w, http.StatusOK, user)
}

// userCreateHandler creates a new user
func (s *ShibuyaAPI) userCreateHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	var req struct {
		Username      string `json:"username"`
		Email         string `json:"email"`
		FullName      string `json:"full_name"`
		PrimaryRoleID *int64 `json:"primary_role_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid JSON"))
		return
	}

	if req.Username == "" {
		s.handleErrors(w, makeBadRequestError("Username is required"))
		return
	}

	user, err := model.CreateUser(req.Username, req.Email, req.FullName, req.PrimaryRoleID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, user)
}

// userUpdateHandler updates a user
func (s *ShibuyaAPI) userUpdateHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	userIDStr := params.ByName("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid user ID"))
		return
	}

	user, err := model.GetUser(userID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	var req struct {
		Email         string `json:"email"`
		FullName      string `json:"full_name"`
		PrimaryRoleID *int64 `json:"primary_role_id"`
		IsActive      *bool  `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid JSON"))
		return
	}

	if req.Email != "" {
		user.Email = req.Email
	}
	if req.FullName != "" {
		user.FullName = req.FullName
	}
	if req.PrimaryRoleID != nil {
		user.PrimaryRoleID = req.PrimaryRoleID
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	if err := user.Update(); err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, user)
}

// userDeleteHandler deletes a user
func (s *ShibuyaAPI) userDeleteHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	userIDStr := params.ByName("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid user ID"))
		return
	}

	user, err := model.GetUser(userID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	if err := user.Delete(); err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, map[string]string{"message": "User deleted successfully"})
}

// User role assignment handlers

// userRolesGetHandler retrieves roles for a specific user
func (s *ShibuyaAPI) userRolesGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	userIDStr := params.ByName("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid user ID"))
		return
	}

	user, err := model.GetUser(userID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	roles, err := model.GetUserRoles(user.Username)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, roles)
}

// userRoleAssignHandler assigns a role to a user
func (s *ShibuyaAPI) userRoleAssignHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	userIDStr := params.ByName("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid user ID"))
		return
	}

	user, err := model.GetUser(userID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	var req struct {
		RoleID    int64  `json:"role_id"`
		ExpiresAt *int64 `json:"expires_at"` // Unix timestamp
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid JSON"))
		return
	}

	if req.RoleID == 0 {
		s.handleErrors(w, makeBadRequestError("Role ID is required"))
		return
	}

	// Get the granting user
	rbacUser := GetRBACUserFromContext(r)
	grantedBy := rbacUser.Name

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0)
		expiresAt = &t
	}

	err = model.AssignRoleToUser(user.Username, req.RoleID, grantedBy, expiresAt)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, map[string]string{"message": "Role assigned successfully"})
}

// userRoleRemoveHandler removes a role from a user
func (s *ShibuyaAPI) userRoleRemoveHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	userIDStr := params.ByName("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid user ID"))
		return
	}

	roleIDStr := params.ByName("role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid role ID"))
		return
	}

	user, err := model.GetUser(userID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	err = model.RemoveRoleFromUser(user.Username, roleID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, map[string]string{"message": "Role removed successfully"})
}

// userPermissionsGetHandler retrieves all permissions for a user
func (s *ShibuyaAPI) userPermissionsGetHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	userIDStr := params.ByName("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		s.handleErrors(w, makeBadRequestError("Invalid user ID"))
		return
	}

	user, err := model.GetUser(userID)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	permissions, err := model.GetUserPermissions(user.Username)
	if err != nil {
		s.handleErrors(w, err)
		return
	}

	s.jsonise(w, http.StatusOK, permissions)
}

// currentUserHandler returns the current user's RBAC information
func (s *ShibuyaAPI) currentUserHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	rbacUser := GetRBACUserFromContext(r)
	if rbacUser == nil {
		s.handleErrors(w, makeUnauthorizedError())
		return
	}

	s.jsonise(w, http.StatusOK, rbacUser)
}