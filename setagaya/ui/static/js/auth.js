// Setagaya Authentication Manager
// Phase 1: Basic RBAC integration with Alpine.js

class AuthManager {
  constructor() {
    this.user = null;
    this.permissions = [];
    this.roles = [];
    this.initialized = false;
  }

  async init() {
    if (this.initialized) return;
    
    try {
      await this.getCurrentUser();
      this.initialized = true;
    } catch (error) {
      console.error('Failed to initialize auth manager:', error);
      // In no-auth mode, we'll set a default user
      if (window.location.pathname !== '/login') {
        this.setDefaultUser();
      }
    }
  }

  async getCurrentUser() {
    try {
      // For Phase 1, we'll check if RBAC endpoints are available
      // If not available, fall back to session-based auth info
      let response;
      try {
        response = await axios.get('/api/rbac/me');
        this.user = response.data;
        this.permissions = response.data.permissions || [];
        this.roles = response.data.roles || [];
      } catch (rbacError) {
        // Fallback to basic session info - extract from template context
        const accountElement = document.querySelector('[data-account]');
        const isAdminElement = document.querySelector('[data-is-admin]');
        
        if (accountElement) {
          this.user = {
            name: accountElement.dataset.account,
            username: accountElement.dataset.account
          };
          
          // Set basic permissions based on admin status
          const isAdmin = isAdminElement && isAdminElement.dataset.isAdmin === 'true';
          if (isAdmin) {
            this.roles = [{ name: 'administrator', id: 1 }];
            this.permissions = this.getAdminPermissions();
          } else {
            this.roles = [{ name: 'user', id: 2 }];
            this.permissions = this.getUserPermissions();
          }
        }
      }
      
      return this.user;
    } catch (error) {
      console.error('Failed to get current user:', error);
      throw error;
    }
  }

  setDefaultUser() {
    // For development with no-auth mode - match template data
    this.user = {
      name: 'setagaya',
      username: 'setagaya'
    };
    this.roles = [{ name: 'administrator', id: 1 }];
    this.permissions = this.getAdminPermissions();
    this.initialized = true;
  }

  getAdminPermissions() {
    // Basic admin permissions for Phase 1
    return [
      { name: 'projects:create' },
      { name: 'projects:read' },
      { name: 'projects:update' },
      { name: 'projects:delete' },
      { name: 'collections:create' },
      { name: 'collections:read' },
      { name: 'collections:update' },
      { name: 'collections:delete' },
      { name: 'plans:create' },
      { name: 'plans:read' },
      { name: 'plans:update' },
      { name: 'plans:delete' },
      { name: 'system:admin' },
      { name: 'users:manage' },
      { name: 'roles:manage' }
    ];
  }

  getUserPermissions() {
    // Basic user permissions for Phase 1
    return [
      { name: 'projects:read' },
      { name: 'collections:read' },
      { name: 'plans:read' }
    ];
  }

  hasPermission(permission) {
    if (!this.initialized) {
      console.warn('AuthManager not initialized, assuming no permission');
      return false;
    }
    
    return this.permissions.some(p => p.name === permission);
  }

  hasRole(roleName) {
    if (!this.initialized) {
      console.warn('AuthManager not initialized, assuming no role');
      return false;
    }
    
    return this.roles.some(r => r.name === roleName);
  }

  isAdmin() {
    return this.hasRole('administrator') || this.hasPermission('system:admin');
  }

  canManageUsers() {
    return this.hasPermission('users:manage') || this.isAdmin();
  }

  canManageRoles() {
    return this.hasPermission('roles:manage') || this.isAdmin();
  }

  logout() {
    this.user = null;
    this.permissions = [];
    this.roles = [];
    this.initialized = false;
    
    // Use form-based logout to maintain compatibility
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = '/logout';
    document.body.appendChild(form);
    form.submit();
  }

  // Helper methods for UI components
  canCreate(resource) {
    return this.hasPermission(`${resource}:create`);
  }

  canRead(resource) {
    return this.hasPermission(`${resource}:read`);
  }

  canUpdate(resource) {
    return this.hasPermission(`${resource}:update`);
  }

  canDelete(resource) {
    return this.hasPermission(`${resource}:delete`);
  }
}

// Global auth manager instance
window.authManager = new AuthManager();

// Initialize on DOM content loaded
document.addEventListener('DOMContentLoaded', () => {
  window.authManager.init().catch(console.error);
});