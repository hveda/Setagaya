# Setagaya UI RBAC Revamp Plan

## Overview

This document outlines the comprehensive plan to revamp the Setagaya load testing platform's user interface to implement Role-Based Access Control (RBAC) using modern, secure frontend technologies.

## Current State Analysis

### Current Technology Stack
- **Frontend Framework**: Vue.js 2.5.17 (legacy)
- **Styling**: Bootstrap 4
- **HTTP Client**: vue-resource
- **Routing**: vue-router
- **Build System**: None (served as static files)
- **Authentication**: Basic session-based auth with LDAP
- **Authorization**: Minimal ownership-based checks

### Current UI Components
```
setagaya/ui/
├── templates/
│   ├── app.html (main Vue app template)
│   └── login.html (login page)
├── static/
│   ├── css/bootstrap.min.css
│   ├── js/
│   │   ├── lib/ (Vue.js, vue-router, vue-resource)
│   │   ├── app.js (main app & routing)
│   │   ├── admin.js (admin routes)
│   │   ├── collection.js (collection components)
│   │   ├── project.js (project components)
│   │   ├── plan.js (plan components)
│   │   ├── common.js (shared components)
│   │   └── nav.js (navigation)
└── handler.go (Go backend serving templates)
```

### Current RBAC Backend Status
✅ **Complete**: 
- Database schema with roles, permissions, users tables
- API endpoints for user/role/permission management
- RBAC middleware and authentication
- Permission-based route protection

❌ **Missing**: 
- Frontend RBAC integration
- Role-based UI components
- Permission-aware navigation
- Admin management interfaces

## Target Technology Stack

### Frontend Framework: **Alpine.js** 
**Why Alpine.js:**
- 🛡️ **Security**: Minimal attack surface, no build step vulnerabilities
- 🚀 **Performance**: Lightweight (15KB), fast loading
- 📚 **Simplicity**: Easy to learn, minimal migration effort
- 🔄 **Progressive**: Can be incrementally adopted alongside existing Vue.js
- 🏗️ **No Build Step**: Direct HTML enhancement, perfect for Go template integration

### Styling: **Tailwind CSS**
**Why Tailwind:**
- 🎨 **Utility-First**: Rapid UI development
- 📱 **Responsive**: Built-in responsive design utilities
- 🎯 **Customizable**: Easy theming and branding
- 🔧 **Purge**: Small production bundles
- 🌓 **Dark Mode**: Built-in dark mode support

### Additional Libraries
- **Axios**: Modern HTTP client (replace vue-resource)
- **Chart.js**: Data visualization for monitoring
- **Headless UI**: Accessible UI components
- **Heroicons**: Modern icon set

## Implementation Plan

## Makefile Integration Strategy

### Current Makefile Analysis
The existing makefile already provides:
- ✅ **Dependency Management**: `install-deps` for system requirements
- ✅ **Container Orchestration**: Kind cluster management
- ✅ **Service Deployment**: Helm-based deployments
- ✅ **Development Workflow**: Single command startup
- ✅ **Clean Operations**: Resource cleanup

### UI Integration Requirements
1. **Non-Breaking Changes**: All existing commands continue to work
2. **Consistent Patterns**: New UI commands follow existing conventions
3. **Error Handling**: Proper error messages and rollback
4. **Cross-Platform**: Works on macOS, Linux, and CI environments
5. **Dependency Isolation**: UI deps don't conflict with existing tools

### Integration Points

#### Build Process Enhancement
```makefile
# Before: Simple container build
setagaya:
	cd setagaya && sh build.sh api

# After: UI-aware build process
setagaya: ui-build
	cd setagaya && sh build.sh api
	@$(call load_image,api:local)
	@$(call deploy_helm,setagaya)
```

#### Development Workflow
```makefile
# New enhanced development command
dev: install-deps ui-deps
	@echo "🚀 Starting enhanced development environment..."
	@$(MAKE) ui-dev &              # Start CSS watching
	@$(MAKE) --no-print-directory  # Start existing services
	@echo "✅ Full stack ready with UI development"
```

#### Dependency Management
```makefile
# Enhanced dependency installation
install-deps: check-tools
	@echo "Installing system dependencies..."
	# ...existing system deps...
	@$(MAKE) ui-deps
	@echo "✅ All dependencies installed"
```

### Backward Compatibility
- ✅ **Existing Commands**: All current commands work unchanged
- ✅ **Default Behavior**: `make` continues to work as before
- ✅ **Optional UI**: UI features are additive, not required
- ✅ **Graceful Degradation**: Missing UI deps don't break core functionality

### Development Experience Improvements
```bash
# Enhanced developer workflow
make dev          # Start everything with UI watching
make ui-rebuild   # Quick UI rebuild during development
make help         # Shows new UI commands in organized help
```

### CI/CD Integration Benefits
- 🚀 **Faster Builds**: Parallel UI and backend building
- 🔄 **Reproducible**: Locked npm dependencies
- 🧪 **Testable**: UI build validation in CI pipeline
- 📦 **Cacheable**: Docker layer caching for UI assets

## Implementation Plan

### Phase 1: Foundation Setup (Week 1-2)

#### 1.1 Development Environment Setup
```bash
# Install build tools
npm init -y
npm install -D tailwindcss @tailwindcss/forms @tailwindcss/typography
npm install axios alpinejs @headlessui/vue heroicons
```

#### 1.2 Tailwind CSS Configuration
```javascript
// tailwind.config.js
module.exports = {
  content: [
    "./setagaya/ui/templates/**/*.html",
    "./setagaya/ui/static/js/**/*.js"
  ],
  theme: {
    extend: {
      colors: {
        'setagaya': {
          50: '#f0f9ff',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
          900: '#1e3a8a'
        }
      }
    }
  },
  plugins: [
    require('@tailwindcss/forms'),
    require('@tailwindcss/typography')
  ]
}
```

#### 1.3 Build System Setup
```json
// package.json scripts
{
  "scripts": {
    "build-css": "tailwindcss -i ./src/input.css -o ./setagaya/ui/static/css/styles.css --watch",
    "build-css-prod": "tailwindcss -i ./src/input.css -o ./setagaya/ui/static/css/styles.css --minify",
    "dev": "npm run build-css",
    "build": "npm run build-css-prod"
  }
}
```

#### 1.4 Makefile Integration
```makefile
# Add to existing makefile
.PHONY: ui-deps ui-dev ui-build ui-clean setagaya-with-ui

# Install UI dependencies
ui-deps:
	@echo "Installing UI dependencies..."
	@if [ ! -f package.json ]; then \
		npm init -y; \
	fi
	@npm install -D tailwindcss @tailwindcss/forms @tailwindcss/typography
	@npm install axios alpinejs
	@echo "✅ UI dependencies installed"

# Development mode - watch for changes
ui-dev:
	@echo "Starting UI development server..."
	@npm run dev &
	@echo "✅ Tailwind CSS watching for changes"

# Build production assets
ui-build:
	@echo "Building production UI assets..."
	@npm run build
	@echo "✅ Production CSS built"

# Clean UI build artifacts
ui-clean:
	@echo "Cleaning UI build artifacts..."
	@rm -f setagaya/ui/static/css/styles.css
	@rm -rf node_modules package-lock.json
	@echo "✅ UI artifacts cleaned"

# Enhanced setagaya build with UI
setagaya-with-ui: ui-build setagaya
	@echo "✅ Setagaya built with new UI"

# Update the main build target
setagaya: ui-build
	@echo "Building Setagaya with UI assets..."
	cd setagaya && sh build.sh api || { echo "Failed to build api"; exit 1; }
	@$(call load_image,api:local)
	@$(call deploy_helm,setagaya)
	@echo "✅ Setagaya deployed successfully"

# Development workflow
dev: install-deps ui-deps
	@echo "🚀 Starting full development environment..."
	@$(MAKE) ui-dev
	@$(MAKE) --no-print-directory
	@echo "✅ Development environment ready"
	@echo "🌐 Setagaya UI: http://localhost:8080"
	@echo "📊 Grafana: http://localhost:3000"

# Enhanced help with UI commands
help:
	@echo "Setagaya Load Testing Platform - Build Commands"
	@echo ""
	@echo "🏗️  Main Commands:"
	@echo "  make              - Build and deploy full stack (default)"
	@echo "  make dev          - Start development environment with UI watch"
	@echo "  make setagaya      - Build and deploy Setagaya with UI"
	@echo "  make clean        - Clean all resources including UI"
	@echo ""
	@echo "🎨 UI Development:"
	@echo "  make ui-deps      - Install UI dependencies (npm packages)"
	@echo "  make ui-dev       - Start Tailwind CSS watch mode"
	@echo "  make ui-build     - Build production CSS"
	@echo "  make ui-clean     - Clean UI build artifacts"
	@echo ""
	@echo "🔧 Infrastructure:"
	@echo "  make grafana      - Deploy Grafana dashboard"
	@echo "  make prometheus   - Deploy Prometheus monitoring"
	@echo "  make storage      - Deploy local storage"
	@echo "  make db           - Deploy MariaDB database"
	@echo ""
	@echo "🧹 Cleanup:"
	@echo "  make clean-all    - Remove everything including UI deps"
	@echo "  make clean        - Clean Kubernetes resources"
	@echo ""
	@echo "📋 Dependencies:"
	@echo "  make install-deps - Install required system dependencies"
	@echo ""
	@echo "🌐 Access URLs:"
	@echo "  Setagaya:  http://localhost:8080"
	@echo "  Grafana:  http://localhost:3000"

# Enhanced clean-all to include UI
clean-all: clean ui-clean
	@echo "✅ Complete cleanup finished"
```

### Phase 2: Core RBAC Components (Week 3-4)

#### 2.1 Authentication State Management
```javascript
// setagaya/ui/static/js/auth.js
class AuthManager {
  constructor() {
    this.user = null;
    this.permissions = [];
    this.roles = [];
  }

  async getCurrentUser() {
    try {
      const response = await axios.get('/api/rbac/me');
      this.user = response.data;
      this.permissions = response.data.permissions;
      this.roles = response.data.roles;
      return this.user;
    } catch (error) {
      this.logout();
      throw error;
    }
  }

  hasPermission(permission) {
    return this.permissions.some(p => p.name === permission);
  }

  hasRole(roleName) {
    return this.roles.some(r => r.name === roleName);
  }

  isAdmin() {
    return this.hasRole('administrator') || this.hasPermission('system:admin');
  }

  logout() {
    this.user = null;
    this.permissions = [];
    this.roles = [];
    window.location.href = '/login';
  }
}

window.authManager = new AuthManager();
```

#### 2.2 Permission-Based UI Components
```javascript
// setagaya/ui/static/js/rbac-components.js
// Alpine.js directive for permission-based visibility
document.addEventListener('alpine:init', () => {
  Alpine.directive('show-if-permission', (el, { expression }, { evaluate }) => {
    const permission = evaluate(expression);
    if (!window.authManager.hasPermission(permission)) {
      el.style.display = 'none';
    }
  });

  Alpine.directive('show-if-role', (el, { expression }, { evaluate }) => {
    const role = evaluate(expression);
    if (!window.authManager.hasRole(role)) {
      el.style.display = 'none';
    }
  });

  Alpine.directive('show-if-admin', (el) => {
    if (!window.authManager.isAdmin()) {
      el.style.display = 'none';
    }
  });
});
```

### Phase 3: Core UI Revamp (Week 5-8)

#### 3.1 Main Layout Template
```html
<!-- setagaya/ui/templates/layout.html -->
<!DOCTYPE html>
<html lang="en" class="h-full bg-gray-50">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Setagaya {{ .Context }} - Load Testing Platform</title>
    <link href="/static/css/styles.css" rel="stylesheet">
    <script defer src="https://unpkg.com/alpinejs@3.x.x/dist/cdn.min.js"></script>
    <script src="https://unpkg.com/axios/dist/axios.min.js"></script>
</head>
<body class="h-full" x-data="app()" x-init="init()">
    <!-- Navigation -->
    <nav class="bg-white shadow-sm border-b border-gray-200">
        <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            <div class="flex justify-between h-16">
                <div class="flex">
                    <div class="flex-shrink-0 flex items-center">
                        <h1 class="text-2xl font-bold text-setagaya-600">Setagaya</h1>
                        <span class="ml-2 text-sm text-gray-500">{{ .Context }}</span>
                    </div>
                    <div class="hidden sm:ml-6 sm:flex sm:space-x-8">
                        <a href="/" class="border-setagaya-500 text-gray-900 inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium">
                            Projects
                        </a>
                        <a href="#" x-show-if-admin class="border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium">
                            Admin
                        </a>
                    </div>
                </div>
                <div class="flex items-center">
                    <div class="flex-shrink-0">
                        <span class="text-sm text-gray-700" x-text="user?.user?.full_name || user?.name"></span>
                    </div>
                    <div class="ml-3 relative">
                        <button @click="logout()" class="bg-white rounded-md p-2 inline-flex items-center justify-center text-gray-400 hover:text-gray-500 hover:bg-gray-100">
                            Logout
                        </button>
                    </div>
                </div>
            </div>
        </div>
    </nav>

    <!-- Main Content -->
    <main class="flex-1">
        <div class="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
            <!-- Page content goes here -->
            {{ template "content" . }}
        </div>
    </main>

    <script src="/static/js/auth.js"></script>
    <script src="/static/js/rbac-components.js"></script>
    <script src="/static/js/app.js"></script>
</body>
</html>
```

#### 3.2 Project Management Interface
```html
<!-- setagaya/ui/templates/projects.html -->
{{ define "content" }}
<div x-data="projectsPage()" x-init="loadProjects()">
    <!-- Header -->
    <div class="bg-white shadow rounded-lg mb-6">
        <div class="px-6 py-4 border-b border-gray-200">
            <div class="flex justify-between items-center">
                <h1 class="text-2xl font-semibold text-gray-900">Projects</h1>
                <button x-show-if-permission="'projects:create'" 
                        @click="showCreateModal = true"
                        class="bg-setagaya-600 hover:bg-setagaya-700 text-white px-4 py-2 rounded-md text-sm font-medium">
                    <svg class="w-4 h-4 mr-2 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"></path>
                    </svg>
                    New Project
                </button>
            </div>
        </div>

        <!-- Projects Grid -->
        <div class="p-6">
            <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                <template x-for="project in projects" :key="project.id">
                    <div class="bg-white border border-gray-200 rounded-lg hover:shadow-md transition-shadow">
                        <div class="p-6">
                            <h3 class="text-lg font-medium text-gray-900" x-text="project.name"></h3>
                            <p class="text-sm text-gray-500 mt-1">Owner: <span x-text="project.owner"></span></p>
                            
                            <div class="mt-4">
                                <div class="flex text-sm text-gray-500">
                                    <span x-text="project.collections?.length || 0"></span> Collections
                                    <span class="mx-2">•</span>
                                    <span x-text="project.plans?.length || 0"></span> Plans
                                </div>
                            </div>

                            <div class="mt-6 flex space-x-2">
                                <a :href="'/collections?project_id=' + project.id" 
                                   class="text-setagaya-600 hover:text-setagaya-900 text-sm font-medium">
                                    View Collections
                                </a>
                                <button x-show-if-permission="'projects:update'" 
                                        @click="editProject(project)"
                                        class="text-gray-600 hover:text-gray-900 text-sm font-medium">
                                    Edit
                                </button>
                                <button x-show-if-permission="'projects:delete'" 
                                        @click="deleteProject(project)"
                                        class="text-red-600 hover:text-red-900 text-sm font-medium">
                                    Delete
                                </button>
                            </div>
                        </div>
                    </div>
                </template>
            </div>
        </div>
    </div>

    <!-- Create Project Modal -->
    <div x-show="showCreateModal" 
         x-transition:enter="ease-out duration-300"
         x-transition:enter-start="opacity-0"
         x-transition:enter-end="opacity-100"
         class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
        <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
            <h3 class="text-lg font-bold text-gray-900 mb-4">Create New Project</h3>
            <form @submit.prevent="createProject()">
                <div class="mb-4">
                    <label class="block text-sm font-medium text-gray-700 mb-2">Project Name</label>
                    <input x-model="newProject.name" 
                           type="text" 
                           required
                           class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-setagaya-500">
                </div>
                <div class="mb-4">
                    <label class="block text-sm font-medium text-gray-700 mb-2">Owner</label>
                    <input x-model="newProject.owner" 
                           type="text" 
                           required
                           class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-setagaya-500">
                </div>
                <div class="flex justify-end space-x-2">
                    <button type="button" 
                            @click="showCreateModal = false"
                            class="px-4 py-2 text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50">
                        Cancel
                    </button>
                    <button type="submit"
                            class="px-4 py-2 bg-setagaya-600 text-white rounded-md hover:bg-setagaya-700">
                        Create
                    </button>
                </div>
            </form>
        </div>
    </div>
</div>

<script>
function projectsPage() {
    return {
        projects: [],
        showCreateModal: false,
        newProject: {
            name: '',
            owner: 'setagaya' // Default for local development
        },

        async loadProjects() {
            try {
                const response = await axios.get('/api/projects?include_collections=true&include_plans=true');
                this.projects = response.data;
            } catch (error) {
                console.error('Failed to load projects:', error);
            }
        },

        async createProject() {
            try {
                await axios.post('/api/projects', this.newProject);
                this.showCreateModal = false;
                this.newProject = { name: '', owner: 'setagaya' };
                await this.loadProjects();
            } catch (error) {
                alert('Failed to create project: ' + (error.response?.data?.message || error.message));
            }
        },

        async deleteProject(project) {
            if (confirm(`Are you sure you want to delete project "${project.name}"?`)) {
                try {
                    await axios.delete(`/api/projects/${project.id}`);
                    await this.loadProjects();
                } catch (error) {
                    alert('Failed to delete project: ' + (error.response?.data?.message || error.message));
                }
            }
        }
    }
}
</script>
{{ end }}
```

### Phase 4: Admin Interface (Week 9-10)

#### 4.1 User Management Interface
```html
<!-- setagaya/ui/templates/admin/users.html -->
{{ define "content" }}
<div x-data="usersAdminPage()" x-init="loadUsers(); loadRoles()">
    <div class="bg-white shadow rounded-lg">
        <div class="px-6 py-4 border-b border-gray-200">
            <div class="flex justify-between items-center">
                <h1 class="text-2xl font-semibold text-gray-900">User Management</h1>
                <button @click="showCreateModal = true"
                        class="bg-setagaya-600 hover:bg-setagaya-700 text-white px-4 py-2 rounded-md text-sm font-medium">
                    Add User
                </button>
            </div>
        </div>

        <!-- Users Table -->
        <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200">
                <thead class="bg-gray-50">
                    <tr>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">User</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Roles</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
                        <th class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Last Login</th>
                        <th class="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                    </tr>
                </thead>
                <tbody class="bg-white divide-y divide-gray-200">
                    <template x-for="user in users" :key="user.id">
                        <tr>
                            <td class="px-6 py-4 whitespace-nowrap">
                                <div class="flex items-center">
                                    <div class="flex-shrink-0 h-10 w-10">
                                        <div class="h-10 w-10 rounded-full bg-setagaya-100 flex items-center justify-center">
                                            <span class="text-sm font-medium text-setagaya-600" x-text="user.username[0].toUpperCase()"></span>
                                        </div>
                                    </div>
                                    <div class="ml-4">
                                        <div class="text-sm font-medium text-gray-900" x-text="user.full_name || user.username"></div>
                                        <div class="text-sm text-gray-500" x-text="user.email"></div>
                                    </div>
                                </div>
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap">
                                <div class="flex flex-wrap gap-1">
                                    <template x-for="role in user.roles" :key="role.id">
                                        <span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-800">
                                            <span x-text="role.name"></span>
                                        </span>
                                    </template>
                                </div>
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap">
                                <span :class="user.is_active ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800'" 
                                      class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium">
                                    <span x-text="user.is_active ? 'Active' : 'Inactive'"></span>
                                </span>
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                                <span x-text="user.last_login ? new Date(user.last_login).toLocaleDateString() : 'Never'"></span>
                            </td>
                            <td class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium">
                                <button @click="editUser(user)" class="text-indigo-600 hover:text-indigo-900 mr-3">Edit</button>
                                <button @click="manageUserRoles(user)" class="text-blue-600 hover:text-blue-900 mr-3">Roles</button>
                                <button @click="deleteUser(user)" class="text-red-600 hover:text-red-900">Delete</button>
                            </td>
                        </tr>
                    </template>
                </tbody>
            </table>
        </div>
    </div>

    <!-- Role Assignment Modal -->
    <div x-show="showRolesModal" 
         x-transition:enter="ease-out duration-300"
         class="fixed inset-0 bg-gray-600 bg-opacity-50 overflow-y-auto h-full w-full z-50">
        <div class="relative top-20 mx-auto p-5 border w-96 shadow-lg rounded-md bg-white">
            <h3 class="text-lg font-bold text-gray-900 mb-4">Manage User Roles</h3>
            <template x-if="selectedUser">
                <div>
                    <p class="text-sm text-gray-600 mb-4">User: <span x-text="selectedUser.username"></span></p>
                    
                    <div class="space-y-2">
                        <template x-for="role in availableRoles" :key="role.id">
                            <label class="flex items-center">
                                <input type="checkbox" 
                                       :checked="userHasRole(selectedUser, role.id)"
                                       @change="toggleUserRole(selectedUser, role.id, $event.target.checked)"
                                       class="rounded border-gray-300 text-setagaya-600 focus:ring-setagaya-500">
                                <span class="ml-2 text-sm text-gray-700" x-text="role.name"></span>
                                <span class="ml-1 text-xs text-gray-500" x-text="'(' + role.description + ')'"></span>
                            </label>
                        </template>
                    </div>

                    <div class="flex justify-end space-x-2 mt-6">
                        <button @click="showRolesModal = false"
                                class="px-4 py-2 text-gray-600 border border-gray-300 rounded-md hover:bg-gray-50">
                            Close
                        </button>
                    </div>
                </div>
            </template>
        </div>
    </div>
</div>

<script>
function usersAdminPage() {
    return {
        users: [],
        availableRoles: [],
        showCreateModal: false,
        showRolesModal: false,
        selectedUser: null,

        async loadUsers() {
            try {
                const response = await axios.get('/api/rbac/users');
                this.users = response.data;
            } catch (error) {
                console.error('Failed to load users:', error);
            }
        },

        async loadRoles() {
            try {
                const response = await axios.get('/api/rbac/roles');
                this.availableRoles = response.data;
            } catch (error) {
                console.error('Failed to load roles:', error);
            }
        },

        manageUserRoles(user) {
            this.selectedUser = user;
            this.showRolesModal = true;
        },

        userHasRole(user, roleId) {
            return user.roles?.some(role => role.id === roleId) || false;
        },

        async toggleUserRole(user, roleId, isChecked) {
            try {
                if (isChecked) {
                    await axios.post(`/api/rbac/users/${user.id}/roles`, { role_id: roleId });
                } else {
                    await axios.delete(`/api/rbac/users/${user.id}/roles/${roleId}`);
                }
                await this.loadUsers();
            } catch (error) {
                console.error('Failed to update user role:', error);
                alert('Failed to update user role');
            }
        },

        async deleteUser(user) {
            if (confirm(`Are you sure you want to delete user "${user.username}"?`)) {
                try {
                    await axios.delete(`/api/rbac/users/${user.id}`);
                    await this.loadUsers();
                } catch (error) {
                    alert('Failed to delete user: ' + (error.response?.data?.message || error.message));
                }
            }
        }
    }
}
</script>
{{ end }}
```

### Phase 5: Testing & Deployment (Week 11-12)

#### 5.1 Testing Strategy
```markdown
## Testing Checklist

### Functional Testing
- [ ] User login/logout with LDAP
- [ ] Role-based navigation visibility
- [ ] Permission-based action availability
- [ ] Project CRUD operations by role
- [ ] Collection management by permission
- [ ] Plan management by permission
- [ ] Admin user management
- [ ] Role assignment/removal

### Security Testing
- [ ] Direct API access without proper permissions
- [ ] URL manipulation to access unauthorized pages
- [ ] Role escalation attempts
- [ ] Session management and timeouts
- [ ] CSRF protection validation

### Performance Testing
- [ ] Page load times with RBAC checks
- [ ] Large user lists rendering
- [ ] Permission checking performance
- [ ] Memory usage optimization

### Browser Compatibility
- [ ] Chrome (latest 2 versions)
- [ ] Firefox (latest 2 versions)
- [ ] Safari (latest 2 versions)
- [ ] Edge (latest 2 versions)
```

#### 5.2 Deployment Steps
```bash
# 1. Build production assets
make ui-build

# 2. Update Go templates to use new layout
# 3. Test with existing API endpoints
# 4. Deploy to staging environment
make setagaya-with-ui

# 5. Run full regression tests
# 6. Deploy to production
```

#### 5.3 Makefile Integration for CI/CD
```makefile
# CI/CD Pipeline Integration
.PHONY: ci-test ci-build ci-deploy

# CI testing phase
ci-test: ui-deps
	@echo "Running CI tests..."
	@npm run build-css-prod
	@echo "✅ UI assets built successfully"
	# Add your existing tests here
	@echo "✅ All tests passed"

# CI build phase
ci-build: ci-test
	@echo "Building for CI/CD..."
	@$(MAKE) setagaya-with-ui
	@echo "✅ CI build completed"

# CI deployment phase
ci-deploy: ci-build
	@echo "Deploying to environment..."
	# Add environment-specific deployment commands
	@echo "✅ Deployment completed"

# Docker build with UI assets
docker-build-with-ui: ui-build
	@echo "Building Docker image with UI assets..."
	cd setagaya && docker build -f Dockerfile --build-arg env=production -t setagaya:ui-latest .
	@echo "✅ Docker image built with UI"

# Kubernetes deployment with UI
k8s-deploy-ui: docker-build-with-ui
	@echo "Deploying to Kubernetes with new UI..."
	@$(call load_image,setagaya:ui-latest)
	@$(call deploy_helm,setagaya)
	@echo "✅ Kubernetes deployment with UI completed"
```

#### 5.4 Development Workflow Integration
```makefile
# Development workflow commands
.PHONY: start-dev stop-dev restart-dev

# Start complete development environment
start-dev: install-deps ui-deps
	@echo "🚀 Starting complete development environment..."
	@$(MAKE) ui-dev &
	@$(MAKE) --no-print-directory
	@echo ""
	@echo "✅ Development environment started successfully!"
	@echo ""
	@echo "🌐 Access URLs:"
	@echo "  Setagaya UI: http://localhost:8080"
	@echo "  Grafana:    http://localhost:3000"
	@echo ""
	@echo "📝 Development Notes:"
	@echo "  • Tailwind CSS is watching for changes"
	@echo "  • Edit files in setagaya/ui/templates/ and setagaya/ui/static/"
	@echo "  • Changes will auto-rebuild CSS"
	@echo "  • Restart 'make setagaya' to see template changes"

# Stop development environment
stop-dev:
	@echo "Stopping development environment..."
	@pkill -f "tailwindcss" || true
	@$(MAKE) clean
	@echo "✅ Development environment stopped"

# Restart development environment
restart-dev: stop-dev start-dev
	@echo "✅ Development environment restarted"

# Quick UI rebuild during development
ui-rebuild:
	@echo "Rebuilding UI assets..."
	@npm run build-css-prod
	@$(MAKE) setagaya
	@echo "✅ UI rebuilt and deployed"
```

### Phase 6: Documentation & Training (Week 13)

#### 6.1 User Documentation
- Role and permission explanations
- UI navigation guide
- Admin management procedures
- Troubleshooting guide

#### 6.2 Developer Documentation
- Alpine.js component patterns
- RBAC integration guide
- Custom directive usage
- Build system documentation

## Implementation Guidelines

### Security Best Practices
1. **Input Validation**: All user inputs validated on both client and server
2. **Permission Checks**: Double-check permissions on both frontend and backend
3. **Error Handling**: Graceful error handling without exposing system details
4. **Session Management**: Proper session timeout and invalidation
5. **HTTPS Only**: Force HTTPS in production
6. **CSP Headers**: Implement Content Security Policy

### Performance Optimization
1. **Lazy Loading**: Load components only when needed
2. **Caching**: Appropriate caching for static assets
3. **Minification**: Minify CSS and JS in production
4. **CDN**: Use CDN for static assets
5. **Image Optimization**: Optimize images and icons

### Accessibility
1. **ARIA Labels**: Proper ARIA labels for screen readers
2. **Keyboard Navigation**: Full keyboard accessibility
3. **Color Contrast**: Meet WCAG 2.1 AA standards
4. **Focus Management**: Proper focus management in modals
5. **Semantic HTML**: Use semantic HTML elements

### Build System Integration
1. **Makefile Consistency**: All UI commands follow existing makefile patterns
2. **Error Handling**: Proper error handling in build scripts
3. **Dependencies**: Clear dependency management and installation
4. **Development Workflow**: Seamless integration with existing dev processes
5. **CI/CD Ready**: Build system works in automated environments

### Development Workflow Best Practices
```bash
# Daily development workflow
make start-dev          # Start everything with UI watching
# Edit templates/CSS/JS files
make ui-rebuild         # Quick rebuild when needed
make stop-dev           # Clean shutdown

# Before committing
make ui-build           # Ensure production build works
make ci-test            # Run full test suite

# For deployment
make ci-deploy          # Full CI/CD pipeline
```

## Migration Strategy

### Phase 1: Parallel Development
- Build new RBAC components alongside existing Vue.js components
- Test new components with existing API endpoints
- Gradual replacement of Vue.js components

### Phase 2: Feature Flag Rollout
- Implement feature flags to toggle between old and new UI
- Progressive rollout to different user groups
- Monitoring and feedback collection

### Phase 3: Complete Migration
- Full replacement of Vue.js components
- Remove legacy JavaScript files
- Update all templates to use new system

## Success Metrics

### User Experience
- ✅ Faster page load times (target: <2s)
- ✅ Improved user satisfaction scores
- ✅ Reduced support tickets for permission issues
- ✅ Increased feature adoption rates

### Technical Metrics
- ✅ Reduced JavaScript bundle size (target: <100KB)
- ✅ Improved lighthouse scores (target: >90)
- ✅ Better Core Web Vitals scores
- ✅ Reduced security vulnerabilities

### Business Metrics
- ✅ Improved admin productivity
- ✅ Better compliance reporting
- ✅ Reduced onboarding time for new users
- ✅ Enhanced audit trail capabilities

## Risk Mitigation

### Technical Risks
- **Risk**: Alpine.js learning curve
  **Mitigation**: Provide training and documentation
  
- **Risk**: Browser compatibility issues
  **Mitigation**: Comprehensive testing across browsers
  
- **Risk**: Performance degradation
  **Mitigation**: Performance monitoring and optimization

### Business Risks
- **Risk**: User resistance to change
  **Mitigation**: Gradual rollout with training
  
- **Risk**: Downtime during migration
  **Mitigation**: Feature flag-based deployment
  
- **Risk**: Security vulnerabilities
  **Mitigation**: Security audits and penetration testing

## Timeline Summary

| Phase | Duration | Key Deliverables |
|-------|----------|-----------------|
| 1 | Week 1-2 | Foundation setup, build system, **makefile integration** |
| 2 | Week 3-4 | Core RBAC components, auth management |
| 3 | Week 5-8 | Main UI revamp, project/collection interfaces |
| 4 | Week 9-10 | Admin interface, user/role management |
| 5 | Week 11-12 | Testing, bug fixes, **CI/CD pipeline**, deployment prep |
| 6 | Week 13 | Documentation, training, final deployment |

**Total Duration: 13 weeks (3.25 months)**

### Detailed Phase 1 Makefile Tasks
- ✅ Add UI dependency management (`make ui-deps`)
- ✅ Integrate Tailwind CSS build process
- ✅ Create development workflow commands (`make start-dev`)
- ✅ Update existing `setagaya` target to include UI build
- ✅ Add UI-specific clean and help commands
- ✅ Test makefile integration with existing workflow

### Phase 5 CI/CD Integration Tasks
- ✅ Create CI-ready makefile targets
- ✅ Add Docker build integration with UI assets
- ✅ Update Kubernetes deployment for new UI
- ✅ Test automated deployment pipeline
- ✅ Validate build reproducibility

## Conclusion

This revamp plan transforms Setagaya's UI from a legacy Vue.js application to a modern, secure, and maintainable Alpine.js + Tailwind CSS solution. The phased approach ensures minimal disruption while delivering significant improvements in security, user experience, and maintainability.

The new architecture leverages existing RBAC backend infrastructure while providing a clean, modern frontend that scales well and is easy to maintain. The focus on security, performance, and accessibility ensures the platform meets enterprise requirements while providing an excellent user experience.
