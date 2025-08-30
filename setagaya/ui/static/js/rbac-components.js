// Setagaya RBAC Components for Alpine.js
// Phase 1: Permission-based UI directives

document.addEventListener('alpine:init', () => {
  // Permission-based visibility directive
  Alpine.directive('show-if-permission', (el, { expression }, { evaluate }) => {
    const permission = evaluate(expression);
    const hasPermission = window.authManager && window.authManager.hasPermission(permission);
    
    if (!hasPermission) {
      el.style.display = 'none';
      el.setAttribute('aria-hidden', 'true');
    } else {
      el.style.display = '';
      el.removeAttribute('aria-hidden');
    }
  });

  // Role-based visibility directive
  Alpine.directive('show-if-role', (el, { expression }, { evaluate }) => {
    const role = evaluate(expression);
    const hasRole = window.authManager && window.authManager.hasRole(role);
    
    if (!hasRole) {
      el.style.display = 'none';
      el.setAttribute('aria-hidden', 'true');
    } else {
      el.style.display = '';
      el.removeAttribute('aria-hidden');
    }
  });

  // Admin-only visibility directive
  Alpine.directive('show-if-admin', (el) => {
    const isAdmin = window.authManager && window.authManager.isAdmin();
    
    if (!isAdmin) {
      el.style.display = 'none';
      el.setAttribute('aria-hidden', 'true');
    } else {
      el.style.display = '';
      el.removeAttribute('aria-hidden');
    }
  });

  // Disable element based on permission
  Alpine.directive('disable-if-no-permission', (el, { expression }, { evaluate }) => {
    const permission = evaluate(expression);
    const hasPermission = window.authManager && window.authManager.hasPermission(permission);
    
    if (!hasPermission) {
      el.disabled = true;
      el.setAttribute('aria-disabled', 'true');
      el.style.opacity = '0.5';
      el.style.cursor = 'not-allowed';
    } else {
      el.disabled = false;
      el.removeAttribute('aria-disabled');
      el.style.opacity = '';
      el.style.cursor = '';
    }
  });
});

// Alpine.js store for global auth state
document.addEventListener('alpine:init', () => {
  Alpine.store('auth', {
    user: null,
    permissions: [],
    roles: [],
    initialized: false,

    async init() {
      if (window.authManager) {
        await window.authManager.init();
        this.user = window.authManager.user;
        this.permissions = window.authManager.permissions;
        this.roles = window.authManager.roles;
        this.initialized = window.authManager.initialized;
      }
    },

    hasPermission(permission) {
      return window.authManager ? window.authManager.hasPermission(permission) : false;
    },

    hasRole(role) {
      return window.authManager ? window.authManager.hasRole(role) : false;
    },

    isAdmin() {
      return window.authManager ? window.authManager.isAdmin() : false;
    },

    canCreate(resource) {
      return this.hasPermission(`${resource}:create`);
    },

    canRead(resource) {
      return this.hasPermission(`${resource}:read`);
    },

    canUpdate(resource) {
      return this.hasPermission(`${resource}:update`);
    },

    canDelete(resource) {
      return this.hasPermission(`${resource}:delete`);
    }
  });
});

// Utility functions for templates
window.shibuyaRBAC = {
  // Check if user has permission for a specific action
  can(permission) {
    return window.authManager && window.authManager.hasPermission(permission);
  },

  // Check if user has a specific role
  hasRole(role) {
    return window.authManager && window.authManager.hasRole(role);
  },

  // Check if user is admin
  isAdmin() {
    return window.authManager && window.authManager.isAdmin();
  },

  // Get current user info
  currentUser() {
    return window.authManager ? window.authManager.user : null;
  },

  // Resource-based permission checks
  canManageProjects() {
    return this.can('projects:create') || this.can('projects:update') || this.can('projects:delete');
  },

  canManageCollections() {
    return this.can('collections:create') || this.can('collections:update') || this.can('collections:delete');
  },

  canManagePlans() {
    return this.can('plans:create') || this.can('plans:update') || this.can('plans:delete');
  },

  canManageUsers() {
    return this.can('users:manage') || this.isAdmin();
  },

  canManageRoles() {
    return this.can('roles:manage') || this.isAdmin();
  }
};

// Make RBAC utilities available globally
window.RBAC = window.shibuyaRBAC;