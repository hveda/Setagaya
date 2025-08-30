// Setagaya Admin Interface - Alpine.js Implementation
// Phase 3: Admin Interface Conversion with RBAC Integration

// Admin Root Component
function adminRootComponent() {
    return {
        activeTab: 'overview',
        userStats: {
            totalUsers: 0,
            activeUsers: 0,
            adminUsers: 0
        },
        systemStats: {
            runningCollections: 0,
            totalProjects: 0,
            systemLoad: 0
        },

        async init() {
            if (!window.authManager.hasPermission('system:admin')) {
                alert('Access denied: Admin privileges required');
                window.location.hash = '#';
                return;
            }
            await this.loadStats();
            console.log('Admin interface initialized');
        },

        async loadStats() {
            try {
                const [userResponse, systemResponse] = await Promise.all([
                    axios.get('/api/rbac/users'),
                    axios.get('/api/admin/stats')
                ]);

                if (userResponse.data && userResponse.data.users) {
                    this.userStats.totalUsers = userResponse.data.users.length;
                    this.userStats.activeUsers = userResponse.data.users.filter(u => u.active).length;
                    this.userStats.adminUsers = userResponse.data.users.filter(u => 
                        u.roles && u.roles.some(r => r.name === 'administrator')
                    ).length;
                }

                if (systemResponse.data) {
                    this.systemStats = { ...this.systemStats, ...systemResponse.data };
                }
            } catch (error) {
                console.error('Failed to load admin stats:', error);
            }
        },

        setActiveTab(tab) {
            this.activeTab = tab;
        }
    };
}

// Collection Admin Component
function collectionAdminComponent() {
    return {
        running_collections: [],
        node_pools: {},
        loading: false,
        error: null,
        syncInterval: null,

        async init() {
            if (!window.authManager.hasPermission('collections:read_all')) {
                this.error = 'Access denied: Collection admin privileges required';
                return;
            }
            
            await this.fetchRunningCollections();
            this.syncInterval = setInterval(() => this.fetchRunningCollections(), 5000);
            console.log('Collection admin initialized');
        },

        destroy() {
            if (this.syncInterval) {
                clearInterval(this.syncInterval);
            }
        },

        async fetchRunningCollections() {
            try {
                this.loading = true;
                const response = await axios.get('/api/admin/collections');
                
                if (response.data) {
                    this.running_collections = response.data.running_collections || [];
                    this.node_pools = response.data.node_pools || {};
                    this.error = null;
                }
            } catch (error) {
                console.error('Failed to fetch running collections:', error);
                this.error = 'Failed to load collection data';
            } finally {
                this.loading = false;
            }
        },

        collectionUrl(collection_id) {
            return `#collections/${collection_id}`;
        },

        formatTime(timestamp) {
            return timestamp ? new Date(timestamp).toLocaleString() : 'N/A';
        },

        getStatusBadgeClass(status) {
            const statusClasses = {
                'running': 'bg-success',
                'completed': 'bg-primary',
                'failed': 'bg-danger',
                'pending': 'bg-warning'
            };
            return statusClasses[status] || 'bg-secondary';
        }
    };
}

// User Management Component
function userManagementComponent() {
    return {
        users: [],
        roles: [],
        selectedUser: null,
        showUserModal: false,
        showRoleModal: false,
        loading: false,
        error: null,
        searchTerm: '',
        
        newUser: {
            username: '',
            full_name: '',
            email: '',
            active: true
        },
        
        newRole: {
            name: '',
            description: '',
            permissions: []
        },

        async init() {
            if (!window.authManager.hasPermission('users:read')) {
                this.error = 'Access denied: User management privileges required';
                return;
            }
            
            await Promise.all([this.loadUsers(), this.loadRoles()]);
            console.log('User management initialized');
        },

        async loadUsers() {
            try {
                this.loading = true;
                const response = await axios.get('/api/rbac/users');
                this.users = response.data.users || [];
            } catch (error) {
                console.error('Failed to load users:', error);
                this.error = 'Failed to load users';
            } finally {
                this.loading = false;
            }
        },

        async loadRoles() {
            try {
                const response = await axios.get('/api/rbac/roles');
                this.roles = response.data.roles || [];
            } catch (error) {
                console.error('Failed to load roles:', error);
            }
        },

        get filteredUsers() {
            if (!this.searchTerm) return this.users;
            return this.users.filter(user => 
                user.username.toLowerCase().includes(this.searchTerm.toLowerCase()) ||
                user.full_name.toLowerCase().includes(this.searchTerm.toLowerCase()) ||
                user.email.toLowerCase().includes(this.searchTerm.toLowerCase())
            );
        },

        async createUser() {
            if (!window.authManager.hasPermission('users:create')) {
                alert('Access denied: Cannot create users');
                return;
            }

            try {
                await axios.post('/api/rbac/users', this.newUser);
                await this.loadUsers();
                this.showUserModal = false;
                this.resetNewUser();
            } catch (error) {
                console.error('Failed to create user:', error);
                alert('Failed to create user');
            }
        },

        async updateUser(user) {
            if (!window.authManager.hasPermission('users:update')) {
                alert('Access denied: Cannot update users');
                return;
            }

            try {
                await axios.put(`/api/rbac/users/${user.id}`, user);
                await this.loadUsers();
            } catch (error) {
                console.error('Failed to update user:', error);
                alert('Failed to update user');
            }
        },

        async deleteUser(user) {
            if (!window.authManager.hasPermission('users:delete')) {
                alert('Access denied: Cannot delete users');
                return;
            }

            if (!confirm(`Are you sure you want to delete user ${user.username}?`)) {
                return;
            }

            try {
                await axios.delete(`/api/rbac/users/${user.id}`);
                await this.loadUsers();
            } catch (error) {
                console.error('Failed to delete user:', error);
                alert('Failed to delete user');
            }
        },

        async assignRole(userId, roleId) {
            if (!window.authManager.hasPermission('users:assign_roles')) {
                alert('Access denied: Cannot assign roles');
                return;
            }

            try {
                await axios.post(`/api/rbac/users/${userId}/roles`, { role_id: roleId });
                await this.loadUsers();
            } catch (error) {
                console.error('Failed to assign role:', error);
                alert('Failed to assign role');
            }
        },

        resetNewUser() {
            this.newUser = {
                username: '',
                full_name: '',
                email: '',
                active: true
            };
        },

        selectUser(user) {
            this.selectedUser = { ...user };
        }
    };
}

// Role Management Component  
function roleManagementComponent() {
    return {
        roles: [],
        permissions: [],
        selectedRole: null,
        showRoleModal: false,
        loading: false,
        error: null,
        
        newRole: {
            name: '',
            description: '',
            permission_ids: []
        },

        async init() {
            if (!window.authManager.hasPermission('roles:read')) {
                this.error = 'Access denied: Role management privileges required';
                return;
            }
            
            await Promise.all([this.loadRoles(), this.loadPermissions()]);
            console.log('Role management initialized');
        },

        async loadRoles() {
            try {
                this.loading = true;
                const response = await axios.get('/api/rbac/roles');
                this.roles = response.data.roles || [];
            } catch (error) {
                console.error('Failed to load roles:', error);
                this.error = 'Failed to load roles';
            } finally {
                this.loading = false;
            }
        },

        async loadPermissions() {
            try {
                const response = await axios.get('/api/rbac/permissions');
                this.permissions = response.data.permissions || [];
            } catch (error) {
                console.error('Failed to load permissions:', error);
            }
        },

        async createRole() {
            if (!window.authManager.hasPermission('roles:create')) {
                alert('Access denied: Cannot create roles');
                return;
            }

            try {
                await axios.post('/api/rbac/roles', this.newRole);
                await this.loadRoles();
                this.showRoleModal = false;
                this.resetNewRole();
            } catch (error) {
                console.error('Failed to create role:', error);
                alert('Failed to create role');
            }
        },

        async updateRole(role) {
            if (!window.authManager.hasPermission('roles:update')) {
                alert('Access denied: Cannot update roles');
                return;
            }

            try {
                await axios.put(`/api/rbac/roles/${role.id}`, role);
                await this.loadRoles();
            } catch (error) {
                console.error('Failed to update role:', error);
                alert('Failed to update role');
            }
        },

        async deleteRole(role) {
            if (!window.authManager.hasPermission('roles:delete')) {
                alert('Access denied: Cannot delete roles');
                return;
            }

            if (!confirm(`Are you sure you want to delete role ${role.name}?`)) {
                return;
            }

            try {
                await axios.delete(`/api/rbac/roles/${role.id}`);
                await this.loadRoles();
            } catch (error) {
                console.error('Failed to delete role:', error);
                alert('Failed to delete role');
            }
        },

        resetNewRole() {
            this.newRole = {
                name: '',
                description: '',
                permission_ids: []
            };
        },

        selectRole(role) {
            this.selectedRole = { ...role };
        }
    };
}

// Admin Routes Configuration
const adminRoutes = {
    'admin': {
        component: 'adminRoot',
        title: 'Admin Dashboard'
    },
    'admin/collections': {
        component: 'collectionAdmin', 
        title: 'Collection Management'
    },
    'admin/users': {
        component: 'userManagement',
        title: 'User Management'
    },
    'admin/roles': {
        component: 'roleManagement',
        title: 'Role Management'
    }
};

// Admin Router Helper
window.adminRouter = {
    currentRoute: null,
    
    navigate(route) {
        this.currentRoute = route;
        window.location.hash = route;
    },
    
    getComponent(route) {
        const routeConfig = adminRoutes[route.replace('#', '')];
        return routeConfig ? routeConfig.component : null;
    },
    
    getTitle(route) {
        const routeConfig = adminRoutes[route.replace('#', '')];
        return routeConfig ? routeConfig.title : 'Admin';
    }
};

// Admin Component Factory
window.createAdminComponent = function(componentType) {
    switch(componentType) {
        case 'adminRoot':
            return adminRootComponent();
        case 'collectionAdmin':
            return collectionAdminComponent();
        case 'userManagement':
            return userManagementComponent();
        case 'roleManagement':
            return roleManagementComponent();
        default:
            return adminRootComponent();
    }
};
