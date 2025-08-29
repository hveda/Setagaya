# Role-Based Access Control (RBAC) System

This document describes the RBAC (Role-Based Access Control) system implemented in Shibuya for fine-grained permission management.

## Overview

The RBAC system provides a flexible way to manage user permissions in Shibuya by using roles, permissions, and user assignments. It maintains backward compatibility with the existing ownership-based system while adding more granular control.

## Core Components

### Roles

Roles are collections of permissions that can be assigned to users. Each role has:
- **ID**: Unique identifier
- **Name**: Human-readable role name  
- **Description**: Detailed description of the role's purpose
- **Permissions**: List of permissions granted by this role

#### Predefined Roles

1. **Administrator**: Full system access including user management and system configuration
2. **Loadtest Project Manager**: Can create and manage loadtest projects and their configurations
3. **Loadtest User**: Can run and view their own loadtests within assigned projects
4. **Monitor User**: Read-only access to monitoring data and test results

### Permissions

Permissions define granular access rights to specific resources and actions. Each permission has:
- **ID**: Unique identifier
- **Name**: Unique permission name (e.g., "projects:create")
- **Resource**: The resource type (e.g., "projects", "collections")
- **Action**: The action allowed (e.g., "create", "read", "update", "delete")
- **Description**: Human-readable description

#### Permission Categories

- **System**: `system:admin`, `system:read`
- **Users**: `users:create`, `users:read`, `users:update`, `users:delete`, `users:assign_roles`
- **Roles**: `roles:create`, `roles:read`, `roles:update`, `roles:delete`
- **Projects**: `projects:create`, `projects:read`, `projects:update`, `projects:delete`, `projects:read_own`, `projects:update_own`
- **Plans**: `plans:create`, `plans:read`, `plans:update`, `plans:delete`, `plans:read_own`, `plans:update_own`
- **Collections**: `collections:create`, `collections:read`, `collections:update`, `collections:delete`, `collections:execute`, `collections:read_own`, `collections:execute_own`
- **Monitoring**: `monitoring:read`, `monitoring:read_all`
- **Files**: `files:upload`, `files:download`, `files:delete`

### Users

Users represent authenticated accounts in the system. Each user has:
- **ID**: Unique identifier
- **Username**: Unique username
- **Email**: Email address
- **Full Name**: Display name
- **Primary Role**: Default role for the user
- **Is Active**: Whether the user account is active
- **Roles**: List of assigned roles

## Database Schema

### Tables

#### `roles`
```sql
CREATE TABLE roles (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    description TEXT,
    created_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

#### `permissions`
```sql
CREATE TABLE permissions (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    resource VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL,
    description TEXT,
    created_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### `role_permissions`
```sql
CREATE TABLE role_permissions (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    role_id INT UNSIGNED NOT NULL,
    permission_id INT UNSIGNED NOT NULL,
    created_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);
```

#### `user_roles`
```sql
CREATE TABLE user_roles (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    role_id INT UNSIGNED NOT NULL,
    granted_by VARCHAR(50) NOT NULL,
    granted_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NULL DEFAULT NULL,
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
);
```

#### `users`
```sql
CREATE TABLE users (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    email VARCHAR(100),
    full_name VARCHAR(100),
    primary_role_id INT UNSIGNED,
    is_active BOOLEAN DEFAULT TRUE,
    last_login TIMESTAMP NULL DEFAULT NULL,
    created_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (primary_role_id) REFERENCES roles(id) ON SET NULL
);
```

## API Endpoints

### Role Management

- **GET** `/api/rbac/roles` - List all roles
- **POST** `/api/rbac/roles` - Create a new role
- **GET** `/api/rbac/roles/:role_id` - Get a specific role
- **PUT** `/api/rbac/roles/:role_id` - Update a role
- **DELETE** `/api/rbac/roles/:role_id` - Delete a role

### Permission Management

- **GET** `/api/rbac/permissions` - List all permissions
- **POST** `/api/rbac/permissions` - Create a new permission (admin only)
- **GET** `/api/rbac/permissions/:permission_id` - Get a specific permission

### User Management

- **GET** `/api/rbac/users` - List all users
- **POST** `/api/rbac/users` - Create a new user
- **GET** `/api/rbac/users/:user_id` - Get a specific user
- **PUT** `/api/rbac/users/:user_id` - Update a user
- **DELETE** `/api/rbac/users/:user_id` - Delete a user

### User Role Assignment

- **GET** `/api/rbac/users/:user_id/roles` - Get user's roles
- **POST** `/api/rbac/users/:user_id/roles` - Assign a role to a user
- **DELETE** `/api/rbac/users/:user_id/roles/:role_id` - Remove a role from a user
- **GET** `/api/rbac/users/:user_id/permissions` - Get user's effective permissions

### Current User

- **GET** `/api/rbac/me` - Get current user's RBAC information

## Usage Examples

### Checking Permissions in Code

```go
// Check if user has a specific permission
hasPermission, err := model.HasPermission("username", "projects:create")
if err != nil {
    return err
}
if !hasPermission {
    return errors.New("insufficient permissions")
}

// Check resource-specific permission
hasResourcePermission, err := model.HasResourcePermission("username", "projects", "read")
if err != nil {
    return err
}
```

### Using RBAC Middleware

```go
// Require specific permission
r.HandlerFunc = s.requirePermission("projects:create")(handler)

// Require specific role
r.HandlerFunc = s.requireRole("administrator")(handler)

// Require admin access
r.HandlerFunc = s.requireAdminRole(handler)

// Use RBAC with ownership checking
r.HandlerFunc = s.projectOwnershipRequired(handler)
```

### Creating Custom Roles

```go
// Create a new role
role, err := model.CreateRole("custom_role", "Custom role for specific team")
if err != nil {
    return err
}

// Assign permissions to the role (requires direct database manipulation)
// In practice, you'd implement a function to manage role-permission assignments
```

## Migration and Setup

### Applying the Migration

1. Apply the database migration:
   ```sql
   SOURCE /path/to/shibuya/db/20241201.sql;
   ```

2. The migration will create all necessary tables and seed initial roles and permissions.

### Default User Assignment

When a user logs in for the first time, they are automatically:
1. Created in the `users` table
2. Assigned the `loadtest_user` role as default
3. Can be promoted to other roles by administrators

## Backward Compatibility

The RBAC system maintains full backward compatibility with the existing ownership model:

- Project ownership through the `owner` field is still respected
- Users can access their own projects and collections as before
- Admin users (configured in `config.json`) retain full access
- Existing middleware continues to work alongside RBAC

## Security Considerations

1. **Principle of Least Privilege**: Users are assigned minimal permissions required for their role
2. **Role Expiration**: User role assignments can have expiration dates
3. **Audit Trail**: All role assignments include granter and timestamp information
4. **Permission Inheritance**: Users inherit all permissions from their assigned roles
5. **Safe Defaults**: New users get the least privileged role by default

## Troubleshooting

### Common Issues

1. **User can't access resources**: Check if user has appropriate role assigned
2. **Permission denied errors**: Verify the required permission exists and is assigned to user's role
3. **Migration fails**: Ensure database user has CREATE and ALTER privileges

### Debugging

1. Check user's effective permissions:
   ```bash
   curl -H "Authorization: Bearer <token>" /api/rbac/me
   ```

2. Verify role assignments:
   ```bash
   curl -H "Authorization: Bearer <token>" /api/rbac/users/<user_id>/roles
   ```

3. List available permissions:
   ```bash
   curl -H "Authorization: Bearer <token>" /api/rbac/permissions
   ```

## Future Enhancements

1. **Dynamic Permission Management**: UI for managing permissions
2. **Group-based Permissions**: Assign permissions to groups instead of individual users
3. **Conditional Permissions**: Context-aware permissions based on resource attributes
4. **Permission Templates**: Predefined permission sets for common use cases
5. **API Rate Limiting**: Per-role rate limiting for API endpoints