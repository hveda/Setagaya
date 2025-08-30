// Setagaya Plan Management - Alpine.js Component
// Converted from Vue.js to Alpine.js for Phase 2

// Plan component for individual plan management
function planComponent(planId) {
    return {
        // Data properties
        planId: planId,
        plan: {},
        loading: true,
        refreshInterval: null,

        // Initialize component
        async init() {
            await this.fetchPlan();
            // Set up auto-refresh
            this.refreshInterval = setInterval(() => {
                this.fetchPlan();
            }, window.SYNC_INTERVAL || 5000);
        },

        // Cleanup when component is destroyed
        destroy() {
            if (this.refreshInterval) {
                clearInterval(this.refreshInterval);
            }
        },

        // Computed properties
        get upload_url() {
            return `plans/${this.plan.id}/files`;
        },

        // Methods
        async fetchPlan() {
            try {
                const response = await axios.get(`/api/plans/${this.planId}`);
                this.plan = response.data;
                this.loading = false;
            } catch (error) {
                console.error('Failed to fetch plan:', error);
                this.loading = false;
                if (error.response?.status !== 401) {
                    alert('Failed to load plan: ' + (error.response?.data?.message || error.message));
                }
            }
        },

        async deletePlan() {
            if (!window.authManager.hasPermission('plans:delete')) {
                alert('You do not have permission to delete plans');
                return;
            }

            const confirmed = confirm("You are going to delete the plan. Continue?");
            if (!confirmed) return;

            try {
                await axios.delete(`/api/plans/${this.planId}`);
                window.location.href = "/";
            } catch (error) {
                alert('Failed to delete plan: ' + (error.response?.data?.message || error.message));
            }
        },

        async deletePlanFile(filename) {
            if (!window.authManager.hasPermission('plans:update')) {
                alert('You do not have permission to delete plan files');
                return;
            }

            const url = encodeURI(`/api/plans/${this.planId}/files?filename=${filename}`);
            try {
                await axios.delete(url);
                alert("File deleted successfully");
                // Refresh plan data
                await this.fetchPlan();
            } catch (error) {
                alert('Failed to delete file: ' + (error.response?.data?.message || error.message));
            }
        },

        // File upload handler
        async handleFileUpload(event) {
            if (!window.authManager.hasPermission('plans:update')) {
                alert('You do not have permission to upload files');
                return;
            }

            const file = event.target.files[0];
            if (!file) return;

            // Validate file type for plan files
            const allowedTypes = ['.csv', '.jmx', '.txt', '.json'];
            const fileExtension = '.' + file.name.split('.').pop().toLowerCase();
            
            if (!allowedTypes.includes(fileExtension)) {
                alert('Invalid file type. Allowed types: ' + allowedTypes.join(', '));
                event.target.value = ''; // Clear the input
                return;
            }

            const formData = new FormData();
            formData.append('planFile', file);

            try {
                await axios.post(`/api/plans/${this.planId}/files`, formData, {
                    headers: {
                        'Content-Type': 'multipart/form-data'
                    }
                });
                alert("File uploaded successfully");
                // Clear the input and refresh plan data
                event.target.value = '';
                await this.fetchPlan();
            } catch (error) {
                alert('Failed to upload file: ' + (error.response?.data?.message || error.message));
                event.target.value = ''; // Clear the input on error
            }
        },

        // Get file download URL
        getFileDownloadUrl(file) {
            return file.filelink || `/api/plans/${this.planId}/files/${file.filename}`;
        },

        // Check if file is a test file (.jmx)
        isTestFile(file) {
            return file.filename && file.filename.toLowerCase().endsWith('.jmx');
        },

        // Get help URL
        getUploadFileHelp() {
            return window.upload_file_help || '#';
        }
    };
}

// Make component globally available
window.planComponent = planComponent;