package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/hveda/setagaya/setagaya/model"
)

const (
	userContextKey = "rbac_user"
)

// RBACUser represents the authenticated user with RBAC information
type RBACUser struct {
	*model.Account
	User        *model.User       `json:"user"`
	Roles       []model.Role      `json:"roles"`
	Permissions []model.Permission `json:"permissions"`
}

// hasPermission checks if the user has a specific permission
func (u *RBACUser) hasPermission(permission string) bool {
	// Admin users have all permissions
	if u.Account.IsAdmin() {
		return true
	}
	
	for _, perm := range u.Permissions {
		if perm.Name == permission {
			return true
		}
	}
	return false
}

// hasResourcePermission checks if the user has permission for a specific resource and action
func (u *RBACUser) hasResourcePermission(resource, action string) bool {
	// Admin users have all permissions
	if u.Account.IsAdmin() {
		return true
	}
	
	for _, perm := range u.Permissions {
		if perm.Resource == resource && perm.Action == action {
			return true
		}
	}
	return false
}

// hasRole checks if the user has a specific role
func (u *RBACUser) hasRole(roleName string) bool {
	for _, role := range u.Roles {
		if role.Name == roleName {
			return true
		}
	}
	return false
}

// isAdmin checks if the user has administrator privileges
func (u *RBACUser) isAdmin() bool {
	return u.hasRole("administrator") || u.hasPermission("system:admin")
}

// canAccessProject checks if user can access a project (either owns it or has appropriate permissions)
func (u *RBACUser) canAccessProject(project *model.Project, action string) bool {
	// Admin can access all projects
	if u.isAdmin() {
		return true
	}

	// Check if user owns the project (backward compatibility)
	if _, ok := u.MLMap[project.Owner]; ok {
		return true
	}

	// Check RBAC permissions
	switch action {
	case "read":
		return u.hasPermission("projects:read") || u.hasPermission("projects:read_own")
	case "update":
		return u.hasPermission("projects:update") || u.hasPermission("projects:update_own")
	case "delete":
		return u.hasPermission("projects:delete")
	case "create":
		return u.hasPermission("projects:create")
	default:
		return false
	}
}

// loadRBACUser creates an RBACUser from the authenticated account
func loadRBACUser(account *model.Account) (*RBACUser, error) {
	rbacUser := &RBACUser{
		Account: account,
	}

	// Get or create user in the database
	user, err := model.GetOrCreateUser(account.Name, "", "")
	if err != nil {
		return nil, err
	}
	rbacUser.User = user

	// Load user roles
	roles, err := model.GetUserRoles(account.Name)
	if err != nil {
		return nil, err
	}
	rbacUser.Roles = roles

	// Load user permissions
	permissions, err := model.GetUserPermissions(account.Name)
	if err != nil {
		return nil, err
	}
	rbacUser.Permissions = permissions

	return rbacUser, nil
}

// rbacRequired middleware that loads RBAC information for authenticated users
func (s *SetagayaAPI) rbacRequired(next httprouter.Handle) httprouter.Handle {
	return s.authRequired(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		account := r.Context().Value(accountKey).(*model.Account)

		rbacUser, err := loadRBACUser(account)
		if err != nil {
			s.handleErrors(w, makeInternalServerError("Failed to load user permissions"))
			return
		}

		// Add RBAC user to context
		ctx := context.WithValue(r.Context(), userContextKey, rbacUser)
		next(w, r.WithContext(ctx), params)
	})
}

// requirePermission middleware that requires a specific permission
func (s *SetagayaAPI) requirePermission(permission string) func(httprouter.Handle) httprouter.Handle {
	return func(next httprouter.Handle) httprouter.Handle {
		return s.rbacRequired(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
			rbacUser := r.Context().Value(userContextKey).(*RBACUser)

			if !rbacUser.hasPermission(permission) {
				s.handleErrors(w, makeNoPermissionErr("Insufficient permissions"))
				return
			}

			next(w, r, params)
		})
	}
}

// requireResourcePermission middleware that requires permission for a specific resource and action
func (s *SetagayaAPI) requireResourcePermission(resource, action string) func(httprouter.Handle) httprouter.Handle {
	return func(next httprouter.Handle) httprouter.Handle {
		return s.rbacRequired(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
			rbacUser := r.Context().Value(userContextKey).(*RBACUser)

			if !rbacUser.hasResourcePermission(resource, action) {
				s.handleErrors(w, makeNoPermissionErr("Insufficient permissions for this resource"))
				return
			}

			next(w, r, params)
		})
	}
}

// requireRole middleware that requires a specific role
func (s *SetagayaAPI) requireRole(roleName string) func(httprouter.Handle) httprouter.Handle {
	return func(next httprouter.Handle) httprouter.Handle {
		return s.rbacRequired(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
			rbacUser := r.Context().Value(userContextKey).(*RBACUser)

			if !rbacUser.hasRole(roleName) {
				s.handleErrors(w, makeNoPermissionErr("Required role not found"))
				return
			}

			next(w, r, params)
		})
	}
}

// requireAdminRole middleware that requires administrator role
func (s *SetagayaAPI) requireAdminRole(next httprouter.Handle) httprouter.Handle {
	return s.rbacRequired(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		rbacUser := r.Context().Value(userContextKey).(*RBACUser)

		if !rbacUser.isAdmin() {
			s.handleErrors(w, makeNoPermissionErr("Administrator role required"))
			return
		}

		next(w, r, params)
	})
}

// projectOwnershipRequired middleware that checks project ownership with RBAC support
func (s *SetagayaAPI) projectOwnershipRequired(next httprouter.Handle) httprouter.Handle {
	return s.rbacRequired(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		rbacUser := r.Context().Value(userContextKey).(*RBACUser)

		projectIDStr := params.ByName("project_id")
		if projectIDStr == "" {
			s.handleErrors(w, makeBadRequestError("Project ID required"))
			return
		}

		projectID, err := strconv.ParseInt(projectIDStr, 10, 64)
		if err != nil {
			s.handleErrors(w, makeBadRequestError("Invalid project ID"))
			return
		}

		project, err := model.GetProject(projectID)
		if err != nil {
			s.handleErrors(w, err)
			return
		}

		// Determine the action based on HTTP method
		action := "read"
		switch r.Method {
		case "PUT", "PATCH":
			action = "update"
		case "DELETE":
			action = "delete"
		case "POST":
			action = "create"
		}

		if !rbacUser.canAccessProject(project, action) {
			s.handleErrors(w, makeProjectOwnershipError())
			return
		}

		next(w, r, params)
	})
}

// collectionOwnershipRequired middleware that checks collection ownership with RBAC support
func (s *SetagayaAPI) collectionOwnershipRequired(next httprouter.Handle) httprouter.Handle {
	return s.rbacRequired(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		rbacUser := r.Context().Value(userContextKey).(*RBACUser)

		collection, err := getCollection(params.ByName("collection_id"))
		if err != nil {
			s.handleErrors(w, err)
			return
		}

		project, err := model.GetProject(collection.ProjectID)
		if err != nil {
			s.handleErrors(w, err)
			return
		}

		// Check if user can access the parent project
		action := "read"
		switch r.Method {
		case "PUT", "PATCH":
			action = "update"
		case "DELETE":
			action = "delete"
		case "POST":
			if strings.Contains(r.URL.Path, "trigger") || strings.Contains(r.URL.Path, "deploy") {
				action = "execute"
			} else {
				action = "create"
			}
		}

		// Check project access first
		if !rbacUser.canAccessProject(project, "read") {
			s.handleErrors(w, makeCollectionOwnershipError())
			return
		}

		// Then check collection-specific permissions
		if !rbacUser.canAccessCollection(collection, action) {
			s.handleErrors(w, makeCollectionOwnershipError())
			return
		}

		next(w, r, params)
	})
}

// canAccessCollection checks if user can access a collection
func (u *RBACUser) canAccessCollection(collection *model.Collection, action string) bool {
	// Admin can access all collections
	if u.isAdmin() {
		return true
	}

	// Check RBAC permissions
	switch action {
	case "read":
		return u.hasPermission("collections:read") || u.hasPermission("collections:read_own")
	case "update":
		return u.hasPermission("collections:update")
	case "delete":
		return u.hasPermission("collections:delete")
	case "create":
		return u.hasPermission("collections:create")
	case "execute":
		return u.hasPermission("collections:execute") || u.hasPermission("collections:execute_own")
	default:
		return false
	}
}

// GetRBACUserFromContext retrieves the RBAC user from request context
func GetRBACUserFromContext(r *http.Request) *RBACUser {
	if user := r.Context().Value(userContextKey); user != nil {
		return user.(*RBACUser)
	}
	return nil
}