// Setagaya Project Management - Alpine.js Components
// Converted from Vue.js to Alpine.js for Phase 2

// Project component for individual project cards
function projectComponent(projectData) {
    return {
        project: projectData,
        creating_collection: false,
        creating_plan: false,
        newCollectionForm: {
            name: '',
            project_id: projectData.id
        },
        newPlanForm: {
            name: '',
            project_id: projectData.id
        },

        // URL helpers
        collectionUrl(collection) {
            return `/collections/${collection.id}`;
        },

        planUrl(plan) {
            return `/plans/${plan.id}`;
        },

        // Collection management
        showNewCollectionModal() {
            this.creating_collection = true;
            this.newCollectionForm.name = '';
        },

        async createCollection() {
            if (!window.authManager.hasPermission('collections:create')) {
                alert('You do not have permission to create collections');
                return;
            }

            try {
                const response = await axios.post('/api/collections', this.newCollectionForm);
                this.creating_collection = false;
                this.newCollectionForm.name = '';
                // Trigger refresh of parent component
                this.$dispatch('collection-created', response.data);
                // Refresh page for now - could be improved with reactive updates
                window.location.reload();
            } catch (error) {
                alert('Failed to create collection: ' + (error.response?.data?.message || error.message));
            }
        },

        // Plan management
        showNewPlanModal() {
            this.creating_plan = true;
            this.newPlanForm.name = '';
        },

        async createPlan() {
            if (!window.authManager.hasPermission('plans:create')) {
                alert('You do not have permission to create plans');
                return;
            }

            try {
                const response = await axios.post('/api/plans', this.newPlanForm);
                this.creating_plan = false;
                this.newPlanForm.name = '';
                // Trigger refresh of parent component
                this.$dispatch('plan-created', response.data);
                // Refresh page for now - could be improved with reactive updates
                window.location.reload();
            } catch (error) {
                alert('Failed to create plan: ' + (error.response?.data?.message || error.message));
            }
        },

        // Project deletion
        async deleteProject() {
            if (!window.authManager.hasPermission('projects:delete')) {
                alert('You do not have permission to delete projects');
                return;
            }

            const confirmed = confirm(`You are going to delete the project "${this.project.name}". Continue?`);
            if (!confirmed) return;

            try {
                await axios.delete(`/api/projects/${this.project.id}`);
                // Trigger refresh of parent component
                this.$dispatch('project-deleted', this.project);
                // Refresh page for now - could be improved with reactive updates
                window.location.reload();
            } catch (error) {
                alert('Failed to delete project: ' + (error.response?.data?.message || error.message));
            }
        }
    }
}

// Projects list component for managing all projects
function projectsComponent() {
    return {
        projects: [],
        creating: false,
        loading: true,
        newProjectForm: {
            name: '',
            owner: 'setagaya' // Default for local development
        },
        refreshInterval: null,

        // Initialize component
        async init() {
            await this.fetchProjects();
            // Set up auto-refresh
            this.refreshInterval = setInterval(() => {
                this.fetchProjects();
            }, window.SYNC_INTERVAL || 5000);
        },

        // Cleanup when component is destroyed
        destroy() {
            if (this.refreshInterval) {
                clearInterval(this.refreshInterval);
            }
        },

        // Fetch projects from API
        async fetchProjects() {
            try {
                const response = await axios.get('/api/projects', {
                    params: {
                        include_collections: true,
                        include_plans: true
                    }
                });
                this.projects = response.data;
                this.loading = false;
            } catch (error) {
                console.error('Failed to fetch projects:', error);
                this.loading = false;
                // Don't show error for unauthorized - might be expected
                if (error.response?.status !== 401) {
                    alert('Failed to load projects: ' + (error.response?.data?.message || error.message));
                }
            }
        },

        // Show create project modal
        showCreateModal() {
            if (!window.authManager.hasPermission('projects:create')) {
                alert('You do not have permission to create projects');
                return;
            }
            this.creating = true;
            this.newProjectForm.name = '';
        },

        // Create new project
        async createProject() {
            if (!this.newProjectForm.name.trim()) {
                alert('Project name is required');
                return;
            }

            try {
                const response = await axios.post('/api/projects', this.newProjectForm);
                this.creating = false;
                this.newProjectForm.name = '';
                await this.fetchProjects(); // Refresh list
            } catch (error) {
                alert('Failed to create project: ' + (error.response?.data?.message || error.message));
            }
        },

        // Handle project created event
        onProjectCreated() {
            this.fetchProjects();
        },

        // Handle project deleted event
        onProjectDeleted() {
            this.fetchProjects();
        },

        // Handle collection created event
        onCollectionCreated() {
            this.fetchProjects();
        },

        // Handle plan created event
        onPlanCreated() {
            this.fetchProjects();
        }
    }
}

// Make components globally available
window.projectComponent = projectComponent;
window.projectsComponent = projectsComponent;
