# Setagaya Production UI Implementation Prompt

## Project Context

You are working on the **Setagaya Load Testing Platform** - a distributed load testing platform that orchestrates JMeter engines in Kubernetes clusters. The platform provides a web UI, REST API, and real-time monitoring for scalable performance testing.

## Current State Analysis

### ✅ **Completed (Phase 1-3 Demos)**
- Alpine.js 3.x integration (replacing Vue.js 2.5.17)
- Tailwind CSS 3.4 with custom Setagaya theme
- Docker multi-stage builds with npm integration
- RBAC API endpoints (4 roles, 35 permissions)
- Demo templates: `phase2-demo.html`, `phase3-demo.html`, `admin-interface.html`

### 🎯 **Implementation Objective**

Transform the current `app.html` template from a Vue.js-based legacy UI to a modern, production-ready Alpine.js + Tailwind CSS interface that:

1. **Maintains full backward compatibility** with existing Go handlers
2. **Integrates seamlessly** with RBAC API endpoints
3. **Provides a modern UX** while preserving all functionality
4. **Implements proper RBAC UI controls** throughout the interface

## Technical Requirements

### 🏗 **Architecture Constraints**

```go
// Current Go handler structure that MUST be preserved
func (u *UI) homeHandler(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
    // This handler serves the main app.html template
    // All existing template variables MUST continue to work
}

// Template variables that MUST be preserved in app.html:
type HomeResp struct {
    Account               string  // User account name
    BackgroundColour      string  // Configurable background color
    Context               string  // Environment context (dev/staging/prod)
    IsAdmin               bool    // Admin status for RBAC
    ResultDashboard       string  // Grafana dashboard URL
    EnableSid             bool    // Session ID feature flag
    EngineHealthDashboard string  // Engine monitoring dashboard URL
    ProjectHome           string  // Project home configuration
    UploadFileHelp        string  // Upload help text
    GCDuration            float64 // Garbage collection interval
}
```

### 🔐 **RBAC Integration Requirements**

The UI must integrate with these existing API endpoints:

```bash
# Role Management
GET    /api/rbac/roles              # Get all roles
POST   /api/rbac/roles              # Create role  
GET    /api/rbac/roles/:role_id     # Get specific role
PUT    /api/rbac/roles/:role_id     # Update role
DELETE /api/rbac/roles/:role_id     # Delete role

# Permission Management  
GET    /api/rbac/permissions        # Get all permissions
POST   /api/rbac/permissions        # Create permission
GET    /api/rbac/permissions/:id    # Get specific permission

# User Management
GET    /api/rbac/users              # Get all users
POST   /api/rbac/users              # Create user
GET    /api/rbac/users/:user_id     # Get specific user
PUT    /api/rbac/users/:user_id     # Update user
DELETE /api/rbac/users/:user_id     # Delete user

# User Role Assignment
GET    /api/rbac/users/:user_id/roles        # Get user roles
POST   /api/rbac/users/:user_id/roles        # Assign role to user
DELETE /api/rbac/users/:user_id/roles/:role_id # Remove role from user
GET    /api/rbac/users/:user_id/permissions  # Get effective permissions
```

### 🎨 **UI Framework Specifications**

**Frontend Stack:**
- **Alpine.js 3.x**: Replace all Vue.js components with Alpine.js equivalents
- **Tailwind CSS 3.4**: Use utility-first classes with custom Setagaya theme
- **Axios**: HTTP client for API calls (already included)
- **Bootstrap compatibility**: Keep essential Bootstrap classes for smooth migration

**Design System:**
```css
/* Custom Setagaya Theme Variables (already implemented) */
:root {
  --setagaya-primary: #2563eb;    /* Blue-600 */
  --setagaya-secondary: #64748b;  /* Slate-500 */
  --setagaya-success: #16a34a;    /* Green-600 */
  --setagaya-warning: #d97706;    /* Amber-600 */
  --setagaya-error: #dc2626;      /* Red-600 */
}

/* Custom Component Classes (already implemented) */
.btn-setagaya { /* Primary button style */ }
.btn-outline-setagaya { /* Outline button style */ }
.card { /* Card component */ }
.card-body { /* Card body padding */ }
```

## 📋 **Implementation Tasks**

### 1. **Transform Main Application UI (app.html)**

**Current Structure to Replace:**
```html
<!-- OLD Vue.js structure -->
<div id="app">
  <script>
    Vue.component('project', { ... })
    new Vue({ el: '#app', data: { ... } })
  </script>
</div>
```

**Target Alpine.js Structure:**
```html
<!-- NEW Alpine.js structure -->
<body x-data="appStore()" x-init="init()">
  <div x-data="projectManager()">
    <!-- Project management interface -->
  </div>
  <div x-data="collectionManager()">
    <!-- Collection management interface -->
  </div>
  <div x-data="adminPanel()" x-show="hasPermission('system:admin')">
    <!-- Admin interface with RBAC -->
  </div>
</body>
```

### 2. **Implement RBAC-Aware Navigation**

```html
<!-- Navigation with permission-based visibility -->
<nav x-data="navigation()">
  <a x-show="hasPermission('projects:read')" href="#projects">Projects</a>
  <a x-show="hasPermission('collections:read')" href="#collections">Collections</a>
  <a x-show="hasPermission('monitoring:read')" href="#monitoring">Monitoring</a>
  <a x-show="hasPermission('system:admin')" href="#admin">Admin</a>
</nav>
```

### 3. **Create Alpine.js Data Stores**

```javascript
// Required Alpine.js stores with RBAC integration
function appStore() {
  return {
    // Global application state
    user: null,
    permissions: [],
    projects: [],
    collections: [],
    
    // RBAC methods
    hasPermission(permission) {
      return this.permissions.includes(permission) || this.isAdmin;
    },
    
    hasRole(role) {
      return this.user?.roles?.includes(role) || false;
    },
    
    // API integration methods
    async loadUserData() {
      // Fetch user permissions and roles
    },
    
    async loadProjects() {
      // Fetch projects based on permissions
    }
  }
}

function projectManager() {
  return {
    // Project CRUD operations
    projects: [],
    selectedProject: null,
    
    async createProject(projectData) {
      // POST /api/projects with RBAC validation
    },
    
    async updateProject(projectId, data) {
      // PUT /api/projects/:id with ownership validation
    }
  }
}

function adminPanel() {
  return {
    // Admin interface state
    roles: [],
    permissions: [],
    users: [],
    
    async loadRoles() {
      // GET /api/rbac/roles
    },
    
    async assignRole(userId, roleId) {
      // POST /api/rbac/users/:user_id/roles
    }
  }
}
```

### 4. **Update Go Route Handlers**

**Required Route Updates:**
```go
// In ui/handler.go - Update route paths to remove .html extensions
func (u *UI) InitRoutes() api.Routes {
    return api.Routes{
        &api.Route{Name: "home", Method: "GET", Path: "/", HandlerFunc: u.homeHandler},
        // Remove demo routes and add production routes
        &api.Route{Name: "admin", Method: "GET", Path: "/admin", HandlerFunc: u.adminHandler},
        &api.Route{Name: "projects", Method: "GET", Path: "/projects", HandlerFunc: u.projectsHandler},
        &api.Route{Name: "collections", Method: "GET", Path: "/collections", HandlerFunc: u.collectionsHandler},
        // ... other production routes
    }
}
```

### 5. **Implement Permission-Based UI Components**

```html
<!-- Project creation button with RBAC -->
<button x-show="hasPermission('projects:create')" 
        @click="createProject()" 
        class="btn-setagaya">
    Create Project
</button>

<!-- Admin-only sections -->
<section x-show="hasPermission('system:admin')" 
         x-data="adminInterface()">
    <!-- Role management UI -->
    <div class="card">
        <div class="card-body">
            <h3>Role Management</h3>
            <template x-for="role in roles" :key="role.id">
                <div class="role-item" x-text="role.name"></div>
            </template>
        </div>
    </div>
</section>
```

## 🔧 **Implementation Guidelines**

### **File Organization:**
```
setagaya/ui/
├── templates/
│   ├── app.html                 # Main production UI (MODIFY)
│   ├── login.html              # Keep as-is
│   ├── phase2-demo.html        # Keep for reference
│   ├── phase3-demo.html        # Keep for reference  
│   └── admin-interface.html    # Keep for reference
├── static/
│   ├── css/
│   │   ├── output.css          # Tailwind compiled CSS
│   │   └── styles.css          # Custom styles
│   └── js/
│       └── app.js              # Alpine.js components (CREATE)
└── handler.go                  # Update routes (MODIFY)
```

### **Migration Strategy:**

1. **Phase 1**: Replace Vue.js template syntax with Alpine.js in `app.html`
2. **Phase 2**: Implement Alpine.js data stores and API integration
3. **Phase 3**: Add RBAC-aware UI components throughout interface
4. **Phase 4**: Update Go handlers to serve proper routes
5. **Phase 5**: Test and validate all functionality

### **Testing Requirements:**

```bash
# All these URLs must work after implementation:
http://localhost:8080/                    # Main app interface
http://localhost:8080/admin              # Admin interface (admin role only)
http://localhost:8080/api/rbac/roles     # RBAC API (must integrate with UI)

# UI must respect these permission patterns:
- Administrator: Full access to all features
- Project Manager: Project/collection management, read-all monitoring
- Load Test User: Own projects/collections only, basic monitoring
- Monitor User: Read-only access to projects, collections, and monitoring
```

### **Backward Compatibility Requirements:**

1. **Template Variables**: All Go template variables in `HomeResp` must continue to work
2. **Static Assets**: CSS/JS files must be served correctly via existing `/static/*` route
3. **Authentication**: Login/logout functionality must remain unchanged
4. **API Integration**: All existing API endpoints must work with new UI

## 🎯 **Success Criteria**

**Functional Requirements:**
- [ ] Main interface loads without Vue.js dependencies
- [ ] RBAC permissions control UI element visibility
- [ ] All CRUD operations work through Alpine.js + API integration
- [ ] Admin interface provides full role/permission management
- [ ] Responsive design works on mobile and desktop

**Technical Requirements:**
- [ ] No JavaScript errors in browser console
- [ ] All API calls succeed with proper authentication
- [ ] Template variables render correctly in Go handlers
- [ ] Static assets load with proper caching headers

**Performance Requirements:**  
- [ ] Initial page load < 2 seconds
- [ ] API responses < 500ms for CRUD operations
- [ ] UI updates without full page reloads
- [ ] Minimal JavaScript bundle size (Alpine.js + Tailwind)

## 📚 **Reference Implementation**

Use the working demo templates as reference:
- `phase2-demo.html`: Alpine.js component patterns
- `phase3-demo.html`: Tailwind CSS styling examples  
- `admin-interface.html`: RBAC integration patterns

The goal is to create a production-ready interface that combines the best patterns from all three demo phases into a cohesive, enterprise-grade application.

---

**Start with transforming the main `app.html` template, then progressively enhance with RBAC integration and modern UX patterns. Maintain strict backward compatibility with the existing Go backend.**
