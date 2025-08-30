// Setagaya Collection Management - Alpine.js Component
// Converted from Vue.js to Alpine.js for Phase 2

// Collection component for individual collection management
function collectionComponent(collectionId) {
    return {
        // Data properties
        collectionId: collectionId,
        collection: {},
        collection_status: {},
        cache: {},
        plan_status: {},
        launched: false,
        triggered: false,
        trigger_in_progress: false,
        stop_in_progress: false,
        purge_in_progress: false,
        showing_log: false,
        showing_engines_detail: false,
        log_content: '',
        log_modal_title: '',
        engines_detail: {},
        upload_url: '',
        loading: true,
        refreshInterval: null,

        // Initialize component
        async init() {
            await this.fetchCollection();
            // Set up auto-refresh
            this.refreshInterval = setInterval(() => {
                this.fetchCollection();
            }, window.SYNC_INTERVAL || 5000);
        },

        // Cleanup when component is destroyed
        destroy() {
            if (this.refreshInterval) {
                clearInterval(this.refreshInterval);
            }
        },

        // Computed properties
        get verb() {
            return this.collection_status.pool_size > 1 ? 'are' : 'is';
        },

        get running_context() {
            return window.running_context || 'local';
        },

        get stopped() {
            return !this.triggered;
        },

        get show_runs() {
            if (!Object.prototype.hasOwnProperty.call(this.collection, 'run_history')) return false;
            if (this.collection.run_history === null) return false;
            return this.collection.run_history.length > 0;
        },

        get can_be_launched() {
            let everything_is_launched = true;
            if (this.collection_status.status) {
                this.collection_status.status.forEach(plan => {
                    if (plan.engines_deployed === 0) {
                        this.launched = false;
                        everything_is_launched = false;
                    }
                });
            }
            
            if (!everything_is_launched) {
                return true;
            }
            
            if (this.can_be_triggered) {
                this.launched = true;
            }
            
            if (!Object.prototype.hasOwnProperty.call(this.collection, 'execution_plans')) {
                return false;
            }
            
            return this.collection.execution_plans.length > 0 && !this.launched;
        },

        get can_be_triggered() {
            const collection_status = this.collection_status.status;
            if (collection_status == null) {
                return false;
            }
            
            let result = collection_status.length > 0;
            collection_status.forEach(plan => {
                result = result && plan.engines_deployed === plan.engines && plan.engines_reachable;
            });
            
            return result;
        },

        get launchable() {
            if (this.can_be_triggered) {
                this.launched = true;
            }
            return this.can_be_launched && !this.launched;
        },

        get triggerable() {
            return this.can_be_triggered && this.launched && this.stopped;
        },

        get stoppable() {
            return this.triggered;
        },

        get purge_tip() {
            let result = true;
            if (this.collection_status.status) {
                this.collection_status.status.forEach(plan => {
                    result = result && (plan.engines_deployed === plan.engines);
                });
            }
            
            if (!result) {
                this.purge_in_progress = false;
            }
            
            return this.purge_in_progress && result;
        },

        get collectionConfigDownloadUrl() {
            return `/api/collections/${this.collectionId}/config`;
        },

        get engine_remaining_time() {
            const engines_detail = this.engines_detail;
            const engine_life_span = window.gcDuration || 60; // Default 60 minutes
            
            if (engines_detail.engines && engines_detail.engines.length > 0) {
                const engine = engines_detail.engines[0];
                const now = Date.now();
                const created_time = new Date(engine.created_time);
                const running_time = (now - created_time) / 1000 / 60;
                return Math.ceil(engine_life_span - running_time);
            }
            
            return engine_life_span;
        },

        get total_engines() {
            let total = 0;
            if (this.collection_status.status) {
                this.collection_status.status.forEach(plan => {
                    total += plan.engines;
                });
            }
            return total;
        },

        // Methods
        updateCache(collection_status) {
            let stopped = true;
            collection_status.forEach(plan_status => {
                plan_status.started_time = new Date(plan_status.started_time);
                stopped = stopped && !plan_status.in_progress;
                this.cache[plan_status.plan_id] = plan_status;
            });
            this.triggered = !stopped;
        },

        async fetchCollection() {
            try {
                // Fetch collection data
                const collectionResponse = await axios.get(`/api/collections/${this.collectionId}`);
                this.collection = collectionResponse.data;

                // Fetch collection status
                const statusResponse = await axios.get(`/api/collections/${this.collectionId}/status`);
                this.collection_status = statusResponse.data;
                this.updateCache(this.collection_status.status);
                
                this.loading = false;
            } catch (error) {
                console.error('Failed to fetch collection:', error);
                this.loading = false;
                if (error.response?.status !== 401) {
                    alert('Failed to load collection: ' + (error.response?.data?.message || error.message));
                }
            }
        },

        planUrl(plan_id) {
            return `/plans/${plan_id}`;
        },

        async launch() {
            if (!window.authManager.hasPermission('collections:execute')) {
                alert('You do not have permission to launch collections');
                return;
            }

            try {
                await axios.post(`/api/collections/${this.collectionId}/deploy`);
                this.launched = true;
                this.purged = false;
            } catch (error) {
                alert('Failed to launch collection: ' + (error.response?.data?.message || error.message));
            }
        },

        async trigger() {
            if (!window.authManager.hasPermission('collections:execute')) {
                alert('You do not have permission to trigger collections');
                return;
            }

            this.trigger_in_progress = true;
            try {
                await axios.post(`/api/collections/${this.collectionId}/trigger`);
                this.triggered = true;
                this.trigger_in_progress = false;
            } catch (error) {
                alert('Failed to trigger collection: ' + (error.response?.data?.message || error.message));
                this.trigger_in_progress = false;
            }
        },

        async stop() {
            if (!window.authManager.hasPermission('collections:execute')) {
                alert('You do not have permission to stop collections');
                return;
            }

            this.stop_in_progress = true;
            try {
                await axios.post(`/api/collections/${this.collectionId}/stop`);
                this.triggered = false;
                this.stop_in_progress = false;
            } catch (error) {
                console.error('Failed to stop collection:', error);
                this.stop_in_progress = false;
            }
        },

        async purge() {
            if (!window.authManager.hasPermission('collections:execute')) {
                alert('You do not have permission to purge collections');
                return;
            }

            this.purge_in_progress = true;
            try {
                await axios.post(`/api/collections/${this.collectionId}/purge`);
                this.launched = false;
                this.triggered = false;
            } catch (error) {
                console.error('Failed to purge collection:', error);
            }
        },

        async deleteCollection() {
            if (!window.authManager.hasPermission('collections:delete')) {
                alert('You do not have permission to delete collections');
                return;
            }

            const confirmed = confirm('You are going to delete the collection. Continue?');
            if (!confirmed) return;

            try {
                await axios.delete(`/api/collections/${this.collectionId}`);
                window.location.href = '/';
            } catch (error) {
                alert('Failed to delete collection: ' + (error.response?.data?.message || error.message));
            }
        },

        calPlanLaunchProgress(plan_id) {
            const status = this.cache[plan_id];
            if (status === undefined) {
                return 0;
            }
            const progress = status.engines_deployed / status.engines;
            return (progress * 100).toFixed(0);
        },

        progressBarStyle(plan_id) {
            const progress = this.calPlanLaunchProgress(plan_id);
            return `width: ${progress * 0.5}%`;
        },

        isPlanReachable(plan_id) {
            const status = this.cache[plan_id];
            if (status === undefined) {
                return false;
            }
            return status.engines_reachable;
        },

        reachableText(plan_id) {
            return this.isPlanReachable(plan_id) ? 'Reachable' : 'Unreachable';
        },

        reachableClass(plan_id) {
            return this.isPlanReachable(plan_id) ? 'progress-bar bg-success' : 'progress-bar bg-danger';
        },

        reachableStyle(plan_id) {
            const status = this.cache[plan_id];
            let style = 'width: 100%';
            
            if (status === undefined || !status.engines_deployed) {
                return style;
            }
            
            return 'width: 50%';
        },

        planStarted(plan) {
            const plan_status = this.cache[plan.plan_id];
            if (plan_status === undefined) {
                return false;
            }
            return plan_status.in_progress;
        },

        runningProgress(plan) {
            if (!this.planStarted(plan)) {
                return '0%';
            }
            
            const plan_status = this.cache[plan.plan_id];
            const started_time = plan_status.started_time;
            const now = new Date();
            const delta = Math.abs(now - started_time);
            const duration = plan.duration * 60 * 1000;
            const progress = Math.min(100, delta / duration * 100);
            
            return progress.toFixed(0) + '%';
        },

        runningProgressStyle(plan) {
            const progress = this.runningProgress(plan);
            return `width: ${progress}`;
        },

        async showEnginesDetail() {
            try {
                const response = await axios.get(`/api/collections/${this.collection.id}/engines_detail`);
                this.showing_engines_detail = true;
                this.engines_detail = response.data;
            } catch (error) {
                alert('Failed to load engines detail: ' + (error.response?.data?.message || error.message));
            }
        },

        async viewPlanLog(plan_id) {
            const url = `/api/collections/${this.collection.id}/logs/${plan_id}`;
            this.log_modal_title = `${this.collection.name}/${plan_id}`;
            
            try {
                const response = await axios.get(url);
                this.showing_log = true;
                this.log_content = response.data.c;
            } catch (error) {
                if (!this.triggered) {
                    alert('The collection has not been triggered!');
                    return;
                }
                console.error('Failed to load plan log:', error);
            }
        },

        runGrafanaUrl(run) {
            // Buffer 1 minute before and after because of time lag in shipping of results
            const start = new Date(run.started_time);
            start.setMinutes(start.getMinutes() - 1);
            const end = new Date(run.end_time);
            
            const result_dashboard = window.result_dashboard || '';
            
            if (end.getTime() <= 0) {
                return `${result_dashboard}?var-runID=${run.id}&from=${start.getTime()}&to=now&refresh=3s`;
            }
            
            end.setMinutes(end.getMinutes() + 1);
            return `${result_dashboard}?var-runID=${run.id}&from=${start.getTime()}&to=${end.getTime()}`;
        },

        hasEngineDashboard() {
            return window.engine_health_dashboard && window.engine_health_dashboard !== '';
        },

        engineHealthGrafanaUrl() {
            return `${window.engine_health_dashboard}?var-collectionID=${this.collectionId}`;
        },

        makeUploadURL(path) {
            switch (path) {
                case 'yaml':
                    this.upload_url = `collections/${this.collection.id}/config`;
                    break;
                case 'data':
                    this.upload_url = `collections/${this.collection.id}/files`;
                    break;
                default:
                    console.log('Wrong upload type selection for making collection upload url');
            }
        },

        async deleteCollectionFile(filename) {
            if (!window.authManager.hasPermission('collections:update')) {
                alert('You do not have permission to delete collection files');
                return;
            }

            const url = encodeURI(`/api/collections/${this.collectionId}/files?filename=${filename}`);
            try {
                await axios.delete(url);
                alert('File deleted successfully');
                // Refresh collection data
                await this.fetchCollection();
            } catch (error) {
                alert('Failed to delete file: ' + (error.response?.data?.message || error.message));
            }
        },

        // File upload helper (to be integrated with upload system)
        async uploadFile(file, type = 'data') {
            if (!window.authManager.hasPermission('collections:update')) {
                alert('You do not have permission to upload files');
                return;
            }

            this.makeUploadURL(type);
            const formData = new FormData();
            formData.append('file', file);

            try {
                await axios.post(`/api/${this.upload_url}`, formData, {
                    headers: {
                        'Content-Type': 'multipart/form-data'
                    }
                });
                alert('File uploaded successfully');
                // Refresh collection data
                await this.fetchCollection();
            } catch (error) {
                alert('Failed to upload file: ' + (error.response?.data?.message || error.message));
            }
        },

        // Time zone helper
        toLocalTZ(timestamp) {
            if (!timestamp) return 'N/A';
            return new Date(timestamp).toLocaleString();
        }
    };
}

// Make component globally available
window.collectionComponent = collectionComponent;
