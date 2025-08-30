use setagaya;

-- Create roles table for RBAC system
CREATE TABLE IF NOT EXISTS roles (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    description TEXT,
    created_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_name (name)
) CHARSET=utf8mb4;

-- Create permissions table for granular access control
CREATE TABLE IF NOT EXISTS permissions (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    resource VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL,
    description TEXT,
    created_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_resource_action (resource, action),
    INDEX idx_name (name)
) CHARSET=utf8mb4;

-- Create role_permissions mapping table
CREATE TABLE IF NOT EXISTS role_permissions (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    role_id INT UNSIGNED NOT NULL,
    permission_id INT UNSIGNED NOT NULL,
    created_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY unique_role_permission (role_id, permission_id),
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    FOREIGN KEY (permission_id) REFERENCES permissions(id) ON DELETE CASCADE,
    INDEX idx_role_id (role_id),
    INDEX idx_permission_id (permission_id)
) CHARSET=utf8mb4;

-- Create user_roles mapping table
CREATE TABLE IF NOT EXISTS user_roles (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    role_id INT UNSIGNED NOT NULL,
    granted_by VARCHAR(50) NOT NULL,
    granted_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NULL DEFAULT NULL,
    UNIQUE KEY unique_user_role (username, role_id),
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE,
    INDEX idx_username (username),
    INDEX idx_role_id (role_id),
    INDEX idx_expires (expires_at)
) CHARSET=utf8mb4;

-- Create users table to track user information
CREATE TABLE IF NOT EXISTS users (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    email VARCHAR(100),
    full_name VARCHAR(100),
    primary_role_id INT UNSIGNED NULL,
    is_active BOOLEAN DEFAULT TRUE,
    last_login TIMESTAMP NULL DEFAULT NULL,
    created_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_username (username),
    INDEX idx_primary_role (primary_role_id),
    INDEX idx_active (is_active),
    FOREIGN KEY (primary_role_id) REFERENCES roles(id) ON DELETE SET NULL
) CHARSET=utf8mb4;

-- Insert initial roles
INSERT INTO roles (name, description) VALUES
('administrator', 'Full system access including user management and system configuration'),
('loadtest_project_manager', 'Can create and manage loadtest projects and their configurations'),
('loadtest_user', 'Can run and view their own loadtests within assigned projects'),
('monitor_user', 'Read-only access to monitoring data and test results');

-- Insert initial permissions
INSERT INTO permissions (name, resource, action, description) VALUES
-- System-wide permissions
('system:admin', 'system', 'admin', 'Full system administration access'),
('system:read', 'system', 'read', 'Read system information and status'),

-- User management permissions
('users:create', 'users', 'create', 'Create new users'),
('users:read', 'users', 'read', 'View user information'),
('users:update', 'users', 'update', 'Update user information'),
('users:delete', 'users', 'delete', 'Delete users'),
('users:assign_roles', 'users', 'assign_roles', 'Assign roles to users'),

-- Role management permissions
('roles:create', 'roles', 'create', 'Create new roles'),
('roles:read', 'roles', 'read', 'View role information'),
('roles:update', 'roles', 'update', 'Update role information'),
('roles:delete', 'roles', 'delete', 'Delete roles'),

-- Project permissions
('projects:create', 'projects', 'create', 'Create new projects'),
('projects:read', 'projects', 'read', 'View project information'),
('projects:update', 'projects', 'update', 'Update project configuration'),
('projects:delete', 'projects', 'delete', 'Delete projects'),
('projects:read_own', 'projects', 'read_own', 'View own projects only'),
('projects:update_own', 'projects', 'update_own', 'Update own projects only'),

-- Plan permissions
('plans:create', 'plans', 'create', 'Create new test plans'),
('plans:read', 'plans', 'read', 'View test plans'),
('plans:update', 'plans', 'update', 'Update test plans'),
('plans:delete', 'plans', 'delete', 'Delete test plans'),
('plans:read_own', 'plans', 'read_own', 'View own test plans only'),
('plans:update_own', 'plans', 'update_own', 'Update own test plans only'),

-- Collection permissions
('collections:create', 'collections', 'create', 'Create new collections'),
('collections:read', 'collections', 'read', 'View collections'),
('collections:update', 'collections', 'update', 'Update collections'),
('collections:delete', 'collections', 'delete', 'Delete collections'),
('collections:execute', 'collections', 'execute', 'Execute load tests'),
('collections:read_own', 'collections', 'read_own', 'View own collections only'),
('collections:execute_own', 'collections', 'execute_own', 'Execute own collections only'),

-- Monitoring permissions
('monitoring:read', 'monitoring', 'read', 'View monitoring data and dashboards'),
('monitoring:read_all', 'monitoring', 'read_all', 'View all monitoring data across projects'),

-- File permissions
('files:upload', 'files', 'upload', 'Upload test files'),
('files:download', 'files', 'download', 'Download test files'),
('files:delete', 'files', 'delete', 'Delete test files');

-- Assign permissions to roles
-- Administrator role gets all permissions
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id 
FROM roles r 
CROSS JOIN permissions p 
WHERE r.name = 'administrator';

-- Loadtest Project Manager role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id 
FROM roles r 
CROSS JOIN permissions p 
WHERE r.name = 'loadtest_project_manager' 
AND p.name IN (
    'system:read',
    'users:read',
    'roles:read',
    'projects:create',
    'projects:read',
    'projects:update',
    'projects:delete',
    'plans:create',
    'plans:read',
    'plans:update',
    'plans:delete',
    'collections:create',
    'collections:read',
    'collections:update',
    'collections:delete',
    'collections:execute',
    'monitoring:read_all',
    'files:upload',
    'files:download',
    'files:delete'
);

-- Loadtest User role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id 
FROM roles r 
CROSS JOIN permissions p 
WHERE r.name = 'loadtest_user' 
AND p.name IN (
    'system:read',
    'users:read',
    'roles:read',
    'projects:read_own',
    'projects:update_own',
    'plans:create',
    'plans:read_own',
    'plans:update_own',
    'plans:delete',
    'collections:create',
    'collections:read_own',
    'collections:execute_own',
    'monitoring:read',
    'files:upload',
    'files:download'
);

-- Monitor User role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id 
FROM roles r 
CROSS JOIN permissions p 
WHERE r.name = 'monitor_user' 
AND p.name IN (
    'system:read',
    'projects:read',
    'plans:read',
    'collections:read',
    'monitoring:read_all',
    'files:download'
);

-- Insert initial test user for local development
-- Note: This is used when "no_auth": true in config_env.json
INSERT INTO users (username, email, full_name, is_active) VALUES
('setagaya', 'setagaya@localhost', 'Setagaya Local Test User', TRUE);

-- Assign administrator role to the local test user
INSERT INTO user_roles (username, role_id, granted_by) 
SELECT 'setagaya', r.id, 'system' 
FROM roles r 
WHERE r.name = 'administrator';

-- Update the user's primary role to administrator
UPDATE users 
SET primary_role_id = (SELECT id FROM roles WHERE name = 'administrator') 
WHERE username = 'setagaya';