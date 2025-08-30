// Setagaya Alpine.js App - Phase 1
// Basic app structure with RBAC integration

// Main Alpine.js app data and methods
function app() {
  return {
    // Application state
    user: null,
    initialized: false,
    
    // Initialize the app
    async init() {
      try {
        // Initialize auth manager
        await window.authManager.init();
        this.user = window.authManager.user;
        this.initialized = true;
        
        // Initialize Alpine auth store
        if (this.$store && this.$store.auth) {
          await this.$store.auth.init();
        }
        
        console.log('Setagaya app initialized with user:', this.user);
      } catch (error) {
        console.error('Failed to initialize app:', error);
        this.initialized = true; // Still mark as initialized to show UI
      }
    },
    
    // Auth methods
    async logout() {
      if (window.authManager) {
        window.authManager.logout();
      }
    },
    
    // Permission helpers for templates
    can(permission) {
      return window.authManager && window.authManager.hasPermission(permission);
    },
    
    hasRole(role) {
      return window.authManager && window.authManager.hasRole(role);
    },
    
    isAdmin() {
      return window.authManager && window.authManager.isAdmin();
    },
    
    // Resource permission helpers
    canCreateProject() {
      return this.can('projects:create');
    },
    
    canManageProjects() {
      return this.can('projects:update') || this.can('projects:delete');
    },
    
    canCreateCollection() {
      return this.can('collections:create');
    },
    
    canManageCollections() {
      return this.can('collections:update') || this.can('collections:delete');
    },
    
    canCreatePlan() {
      return this.can('plans:create');
    },
    
    canManagePlans() {
      return this.can('plans:update') || this.can('plans:delete');
    },
    
    canManageUsers() {
      return this.can('users:manage') || this.isAdmin();
    },
    
    canManageRoles() {
      return this.can('roles:manage') || this.isAdmin();
    }
  }
}

// Make app function globally available
window.shibuyaApp = app;

// Initialize Alpine.js when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
  console.log('Setagaya Alpine.js app loading...');
});

// For backward compatibility during transition
// Keep some Vue.js constants that might be referenced
if (typeof SYNC_INTERVAL === 'undefined') {
  window.SYNC_INTERVAL = 5000; // 5 seconds
}

if (typeof enable_sid === 'undefined') {
  window.enable_sid = false;
}