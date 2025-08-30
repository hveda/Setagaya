# Copilot Agent Prompt: Setagaya UI RBAC Revamp Implementation

## Context & Mission

You are tasked with implementing the **Setagaya UI RBAC Revamp** - transforming a legacy Vue.js interface into a modern, secure Alpine.js + Tailwind CSS system with comprehensive Role-Based Access Control integration.

## Project Status: Ready for Implementation

### ✅ **Backend RBAC System - FULLY OPERATIONAL**
- **Database Schema**: 5 RBAC tables with 4 roles, 35 permissions deployed
- **API Endpoints**: 20+ RBAC endpoints tested and working (`/api/rbac/*`)
- **Authentication**: Session-based auth with no-auth mode for development
- **Permissions**: Granular permissions across projects, collections, plans, files, monitoring
- **Security**: Distroless containers, secure build pipeline validated

### ✅ **Infrastructure - PRODUCTION READY**
- **Kubernetes**: All services running in setagaya-executors namespace
- **Build System**: Makefile with Podman support and UI integration ready
- **Container Security**: Distroless images, static compilation, nonroot execution
- **Database**: MariaDB with RBAC schema and test data populated

### 📋 **Implementation Plan Reference**
Location: `/Users/herilife/Learn/Setagaya/docs/ui-rbac-revamp-plan.md`
- **13-week phased approach** with detailed technical specifications
- **Complete makefile integration strategy** for build system
- **Modern tech stack**: Alpine.js + Tailwind CSS + Axios
- **Security-first design** with permission-based UI components

## Your Implementation Task

Transform the current Vue.js UI located in `setagaya/ui/` to implement the comprehensive RBAC revamp plan.

### Primary Objectives

1. **Replace Legacy Frontend** 
   - Current: Vue.js 2.5.17 + Bootstrap 4 + vue-resource
   - Target: Alpine.js + Tailwind CSS + Axios

2. **Implement RBAC UI Integration**
   - Permission-based component visibility
   - Role-aware navigation and actions
   - Admin user/role management interfaces

3. **Enhance Build System**
   - Integrate UI build process with existing makefile
   - Development workflow with CSS watching
   - Production-ready asset pipeline

4. **Maintain Security Standards**
   - Permission validation on frontend and backend
   - Secure session handling
   - Input validation and error handling

## Current File Structure

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

## Implementation Priority

### Phase 1: Foundation (IMMEDIATE - Week 1-2)
```bash
# Your first tasks:
1. Set up Tailwind CSS build system with makefile integration
2. Create Alpine.js auth management system
3. Build permission-based UI directive system
4. Test with existing RBAC API endpoints
```

### Phase 2: Core UI (Week 3-6)
```bash
# Transform existing components:
1. Convert project.js → Alpine.js project management
2. Convert collection.js → Alpine.js collection interface  
3. Convert plan.js → Alpine.js plan management
4. Update navigation with role-based visibility
```

### Phase 3: Admin Interface (Week 7-8)
```bash
# New admin features:
1. User management interface (/api/rbac/users)
2. Role assignment interface (/api/rbac/users/:id/roles)
3. Permission viewing (/api/rbac/permissions)
4. System administration dashboard
```

## Technical Requirements

### 🛡️ **Security Implementation**
```javascript
// Required: Permission-based UI components
Alpine.directive('show-if-permission', (el, { expression }, { evaluate }) => {
    const permission = evaluate(expression);
    if (!window.authManager.hasPermission(permission)) {
        el.style.display = 'none';
    }
});

// Example usage in templates:
<button x-show-if-permission="'projects:create'" @click="createProject()">
    New Project
</button>
```

### 🏗️ **Build System Integration**
```makefile
# Required makefile targets to implement:
ui-deps:          # Install Tailwind CSS and Alpine.js
ui-dev:           # Start CSS watching for development
ui-build:         # Build production assets
setagaya: ui-build # Enhanced main build target
dev: ui-dev       # Enhanced development workflow
```

### 🎨 **Design System**
```css
/* Required Tailwind configuration */
colors: {
  'setagaya': {
    50: '#f0f9ff',   500: '#3b82f6',
    600: '#2563eb',  700: '#1d4ed8',
    900: '#1e3a8a'
  }
}
```

## RBAC API Integration

### Available Endpoints (Already Working)
```javascript
// User Management
GET    /api/rbac/users              // List all users
POST   /api/rbac/users              // Create user
GET    /api/rbac/users/:id          // Get user details
PUT    /api/rbac/users/:id          // Update user
DELETE /api/rbac/users/:id          // Delete user

// Role Management
GET    /api/rbac/roles              // List all roles
POST   /api/rbac/roles              // Create role
GET    /api/rbac/roles/:id          // Get role details
PUT    /api/rbac/roles/:id          // Update role
DELETE /api/rbac/roles/:id          // Delete role

// User-Role Assignment
GET    /api/rbac/users/:id/roles         // Get user roles
POST   /api/rbac/users/:id/roles         // Assign role to user
DELETE /api/rbac/users/:id/roles/:role_id // Remove role from user
GET    /api/rbac/users/:id/permissions   // Get user permissions

// Permission Management
GET    /api/rbac/permissions        // List all permissions
```

### Authentication Context
```javascript
// Current authentication (no-auth mode for development)
// Default user: "setagaya" with administrator role
// Session-based auth via cookies
// All RBAC endpoints accessible at localhost:8080 (when port-forwarded)
```

## Expected Deliverables

### 1. **Updated File Structure**
```
setagaya/ui/
├── templates/
│   ├── layout.html          # New: Base layout with Alpine.js
│   ├── projects.html        # New: Project management interface
│   ├── collections.html     # New: Collection management
│   ├── admin/
│   │   ├── users.html       # New: User management
│   │   └── roles.html       # New: Role management
├── static/
│   ├── css/
│   │   └── styles.css       # New: Compiled Tailwind CSS
│   ├── js/
│   │   ├── auth.js          # New: Authentication manager
│   │   ├── rbac-components.js # New: RBAC Alpine.js directives
│   │   └── app.js           # Updated: Main Alpine.js app
├── package.json             # New: NPM dependencies
├── tailwind.config.js       # New: Tailwind configuration
└── handler.go               # Updated: Go template handler
```

### 2. **Enhanced Makefile**
```makefile
# New targets you must implement:
make ui-deps     # Install UI dependencies
make ui-dev      # Development with CSS watching  
make ui-build    # Production CSS build
make dev         # Enhanced development workflow
make help        # Updated help with UI commands
```

### 3. **Working RBAC UI Features**
- ✅ Login with role-based navigation
- ✅ Permission-controlled button/menu visibility
- ✅ Project CRUD with proper permission checks
- ✅ Collection management with role restrictions
- ✅ Admin user management interface
- ✅ Role assignment functionality

## Development Environment

### Quick Start Commands
```bash
# Test current RBAC API (already working):
kubectl port-forward -n setagaya-executors service/setagaya-api-local 8080:8080
curl http://localhost:8080/api/rbac/roles   # Should return 4 roles

# Development setup (after your implementation):
make dev                    # Start full development environment
# Access: http://localhost:8080 (new UI)
# Access: http://localhost:3000 (Grafana)
```

### Database Access (for testing)
```bash
# Access RBAC database for verification:
kubectl exec -n setagaya-executors deployment/db -- mariadb -u root -proot setagaya
> SELECT username, full_name FROM users;
> SELECT name, description FROM roles;
```

## Success Criteria

### Functional Requirements
- [ ] All existing Vue.js functionality preserved in Alpine.js
- [ ] RBAC permissions properly enforced in UI
- [ ] Admin interface for user/role management working
- [ ] Build system integrated with existing makefile
- [ ] Production deployment pipeline functional

### Technical Requirements  
- [ ] Page load time < 2 seconds
- [ ] JavaScript bundle < 100KB
- [ ] All major browsers supported
- [ ] WCAG 2.1 AA accessibility compliance
- [ ] Security: No client-side permission bypasses

### Integration Requirements
- [ ] Seamless makefile integration
- [ ] Existing API endpoints unchanged
- [ ] Database schema untouched
- [ ] Container security maintained
- [ ] Development workflow preserved

## Key Implementation Notes

### 🚨 **Critical Requirements**
1. **Backward Compatibility**: Existing API endpoints must continue working
2. **Security First**: Every UI action must be validated by backend permissions
3. **Build Integration**: Must work with existing makefile and container pipeline
4. **No Breaking Changes**: Current deployment process should remain functional

### 🎯 **Focus Areas**
1. **Start with Phase 1**: Foundation setup is critical for everything else
2. **Test Early**: Validate RBAC API integration immediately
3. **Incremental Migration**: Replace Vue.js components one by one
4. **Documentation**: Update all setup and development docs

### 🔧 **Development Tips**
1. **Use Existing RBAC Data**: 4 roles and 35 permissions already configured
2. **Test with Default User**: "setagaya" user has administrator role
3. **Port Forward for Testing**: Use kubectl port-forward for API access
4. **Follow Existing Patterns**: Maintain current URL structure and user flows

## Reference Implementation Examples

The ui-rbac-revamp-plan.md contains complete code examples for:
- Alpine.js authentication manager
- Permission-based UI directives  
- Modern HTML templates with Tailwind CSS
- Makefile integration patterns
- Admin interface components

## Validation Commands

After implementation, these should work:
```bash
make dev                                    # Start development
curl http://localhost:8080/api/rbac/roles   # API accessible  
# Browse to http://localhost:8080           # New UI loads
# Login as "setagaya"                       # Default admin user
# Manage users/roles                        # Admin interface works
```

---

**Your mission**: Transform this legacy Vue.js UI into a modern, secure, RBAC-integrated interface that leverages our fully operational backend system. The plan is detailed, the backend is ready, now bring the vision to life! 🚀
