// Setagaya Real-time Updates System - Phase 3
// WebSocket/SSE Integration for Live Status Monitoring

// Real-time Connection Manager
class RealtimeManager {
    constructor() {
        this.connections = new Map();
        this.eventSources = new Map();
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 1000;
        this.listeners = new Map();
        this.isOnline = navigator.onLine;
        
        this.setupNetworkHandlers();
        console.log('Real-time manager initialized');
    }

    setupNetworkHandlers() {
        window.addEventListener('online', () => {
            this.isOnline = true;
            this.reconnectAll();
        });

        window.addEventListener('offline', () => {
            this.isOnline = false;
            this.closeAll();
        });
    }

    // Server-Sent Events (SSE) connection for collection monitoring
    connectToCollectionUpdates(collectionId) {
        if (!this.isOnline) {
            console.warn('Cannot connect: offline');
            return null;
        }

        const connectionKey = `collection_${collectionId}`;
        
        if (this.eventSources.has(connectionKey)) {
            return this.eventSources.get(connectionKey);
        }

        const eventSource = new EventSource(`/api/collections/${collectionId}/stream`);
        
        eventSource.onopen = () => {
            console.log(`Connected to collection ${collectionId} updates`);
            this.reconnectAttempts = 0;
            this.emit(`collection_${collectionId}_connected`);
        };

        eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                this.emit(`collection_${collectionId}_update`, data);
            } catch (error) {
                console.error('Failed to parse collection update:', error);
            }
        };

        eventSource.onerror = (error) => {
            console.error(`Collection ${collectionId} stream error:`, error);
            this.emit(`collection_${collectionId}_error`, error);
            
            if (eventSource.readyState === EventSource.CLOSED) {
                this.eventSources.delete(connectionKey);
                this.scheduleReconnect(connectionKey, () => this.connectToCollectionUpdates(collectionId));
            }
        };

        // Custom event handlers for different message types
        eventSource.addEventListener('status', (event) => {
            const data = JSON.parse(event.data);
            this.emit(`collection_${collectionId}_status`, data);
        });

        eventSource.addEventListener('metrics', (event) => {
            const data = JSON.parse(event.data);
            this.emit(`collection_${collectionId}_metrics`, data);
        });

        eventSource.addEventListener('error', (event) => {
            const data = JSON.parse(event.data);
            this.emit(`collection_${collectionId}_execution_error`, data);
        });

        this.eventSources.set(connectionKey, eventSource);
        return eventSource;
    }

    // Global system updates stream
    connectToSystemUpdates() {
        if (!this.isOnline) {
            console.warn('Cannot connect: offline');
            return null;
        }

        const connectionKey = 'system_updates';
        
        if (this.eventSources.has(connectionKey)) {
            return this.eventSources.get(connectionKey);
        }

        const eventSource = new EventSource('/api/system/stream');
        
        eventSource.onopen = () => {
            console.log('Connected to system updates');
            this.reconnectAttempts = 0;
            this.emit('system_connected');
        };

        eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                this.emit('system_update', data);
            } catch (error) {
                console.error('Failed to parse system update:', error);
            }
        };

        eventSource.onerror = (error) => {
            console.error('System stream error:', error);
            this.emit('system_error', error);
            
            if (eventSource.readyState === EventSource.CLOSED) {
                this.eventSources.delete(connectionKey);
                this.scheduleReconnect(connectionKey, () => this.connectToSystemUpdates());
            }
        };

        this.eventSources.set(connectionKey, eventSource);
        return eventSource;
    }

    // Admin updates stream for admin interface
    connectToAdminUpdates() {
        if (!window.authManager.hasPermission('system:admin')) {
            console.warn('Access denied: Admin privileges required');
            return null;
        }

        if (!this.isOnline) {
            console.warn('Cannot connect: offline');
            return null;
        }

        const connectionKey = 'admin_updates';
        
        if (this.eventSources.has(connectionKey)) {
            return this.eventSources.get(connectionKey);
        }

        const eventSource = new EventSource('/api/admin/stream');
        
        eventSource.onopen = () => {
            console.log('Connected to admin updates');
            this.reconnectAttempts = 0;
            this.emit('admin_connected');
        };

        eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                this.emit('admin_update', data);
            } catch (error) {
                console.error('Failed to parse admin update:', error);
            }
        };

        eventSource.addEventListener('user_activity', (event) => {
            const data = JSON.parse(event.data);
            this.emit('admin_user_activity', data);
        });

        eventSource.addEventListener('collection_activity', (event) => {
            const data = JSON.parse(event.data);
            this.emit('admin_collection_activity', data);
        });

        eventSource.onerror = (error) => {
            console.error('Admin stream error:', error);
            this.emit('admin_error', error);
            
            if (eventSource.readyState === EventSource.CLOSED) {
                this.eventSources.delete(connectionKey);
                this.scheduleReconnect(connectionKey, () => this.connectToAdminUpdates());
            }
        };

        this.eventSources.set(connectionKey, eventSource);
        return eventSource;
    }

    disconnectFromCollection(collectionId) {
        const connectionKey = `collection_${collectionId}`;
        const eventSource = this.eventSources.get(connectionKey);
        
        if (eventSource) {
            eventSource.close();
            this.eventSources.delete(connectionKey);
            console.log(`Disconnected from collection ${collectionId} updates`);
        }
    }

    disconnectFromSystem() {
        const eventSource = this.eventSources.get('system_updates');
        if (eventSource) {
            eventSource.close();
            this.eventSources.delete('system_updates');
            console.log('Disconnected from system updates');
        }
    }

    disconnectFromAdmin() {
        const eventSource = this.eventSources.get('admin_updates');
        if (eventSource) {
            eventSource.close();
            this.eventSources.delete('admin_updates');
            console.log('Disconnected from admin updates');
        }
    }

    scheduleReconnect(connectionKey, reconnectFn) {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error(`Max reconnection attempts reached for ${connectionKey}`);
            this.emit(`${connectionKey}_max_retries_exceeded`);
            return;
        }

        this.reconnectAttempts++;
        const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1); // Exponential backoff

        console.log(`Scheduling reconnection attempt ${this.reconnectAttempts} for ${connectionKey} in ${delay}ms`);
        
        setTimeout(() => {
            if (this.isOnline) {
                console.log(`Attempting to reconnect ${connectionKey}`);
                reconnectFn();
            }
        }, delay);
    }

    reconnectAll() {
        console.log('Reconnecting all connections...');
        // Clear existing connections
        this.closeAll();
        
        // Emit reconnection event so components can re-establish their connections
        this.emit('reconnect_all');
    }

    closeAll() {
        console.log('Closing all real-time connections');
        
        for (const [, eventSource] of this.eventSources) {
            eventSource.close();
        }
        
        this.eventSources.clear();
        this.connections.clear();
    }

    // Event system for components to listen to real-time updates
    on(event, callback) {
        if (!this.listeners.has(event)) {
            this.listeners.set(event, new Set());
        }
        this.listeners.get(event).add(callback);
    }

    off(event, callback) {
        if (this.listeners.has(event)) {
            this.listeners.get(event).delete(callback);
        }
    }

    emit(event, data = null) {
        if (this.listeners.has(event)) {
            for (const callback of this.listeners.get(event)) {
                try {
                    callback(data);
                } catch (error) {
                    console.error(`Error in event listener for ${event}:`, error);
                }
            }
        }
    }

    // Health check for connections
    getConnectionStatus() {
        const status = {};
        
        for (const [key, eventSource] of this.eventSources) {
            status[key] = {
                readyState: eventSource.readyState,
                url: eventSource.url,
                isConnected: eventSource.readyState === EventSource.OPEN
            };
        }
        
        return {
            isOnline: this.isOnline,
            reconnectAttempts: this.reconnectAttempts,
            connections: status
        };
    }

    // Cleanup method
    destroy() {
        this.closeAll();
        this.listeners.clear();
        console.log('Real-time manager destroyed');
    }
}

// Real-time Collection Monitor Component
function realtimeCollectionComponent() {
    return {
        collectionId: null,
        status: 'disconnected',
        metrics: {},
        errors: [],
        connectionStatus: 'disconnected',
        lastUpdate: null,
        autoReconnect: true,

        init() {
            this.setupEventListeners();
            console.log('Real-time collection component initialized');
        },

        setupEventListeners() {
            // Collection-specific events
            window.realtimeManager.on(`collection_${this.collectionId}_connected`, () => {
                this.connectionStatus = 'connected';
                this.status = 'monitoring';
            });

            window.realtimeManager.on(`collection_${this.collectionId}_update`, (data) => {
                this.handleUpdate(data);
            });

            window.realtimeManager.on(`collection_${this.collectionId}_status`, (data) => {
                this.handleStatusUpdate(data);
            });

            window.realtimeManager.on(`collection_${this.collectionId}_metrics`, (data) => {
                this.handleMetricsUpdate(data);
            });

            window.realtimeManager.on(`collection_${this.collectionId}_execution_error`, (data) => {
                this.handleError(data);
            });

            window.realtimeManager.on(`collection_${this.collectionId}_error`, (error) => {
                this.connectionStatus = 'error';
                this.handleConnectionError(error);
            });

            // Global reconnection event
            window.realtimeManager.on('reconnect_all', () => {
                if (this.autoReconnect && this.collectionId) {
                    this.connect();
                }
            });
        },

        connect() {
            if (!this.collectionId) {
                console.error('Cannot connect: No collection ID specified');
                return;
            }

            this.connectionStatus = 'connecting';
            window.realtimeManager.connectToCollectionUpdates(this.collectionId);
        },

        disconnect() {
            if (this.collectionId) {
                window.realtimeManager.disconnectFromCollection(this.collectionId);
                this.connectionStatus = 'disconnected';
                this.status = 'stopped';
            }
        },

        handleUpdate(data) {
            this.lastUpdate = new Date().toISOString();
            
            if (data.type === 'status') {
                this.status = data.status;
            } else if (data.type === 'metrics') {
                this.metrics = { ...this.metrics, ...data.metrics };
            }
        },

        handleStatusUpdate(data) {
            this.status = data.status;
            this.lastUpdate = new Date().toISOString();
            
            // Emit event for parent components
            this.$dispatch('collection-status-changed', {
                collectionId: this.collectionId,
                status: data.status,
                data: data
            });
        },

        handleMetricsUpdate(data) {
            this.metrics = { ...this.metrics, ...data };
            this.lastUpdate = new Date().toISOString();
            
            // Emit event for parent components
            this.$dispatch('collection-metrics-updated', {
                collectionId: this.collectionId,
                metrics: data
            });
        },

        handleError(error) {
            this.errors.push({
                timestamp: new Date().toISOString(),
                message: error.message || 'Unknown error',
                details: error
            });
            
            // Keep only last 10 errors
            if (this.errors.length > 10) {
                this.errors = this.errors.slice(-10);
            }
        },

        handleConnectionError(error) {
            console.error('Collection real-time connection error:', error);
            
            if (this.autoReconnect) {
                this.connectionStatus = 'reconnecting';
            }
        },

        formatMetric(value, type = 'number') {
            if (type === 'duration' && typeof value === 'number') {
                return `${value}ms`;
            } else if (type === 'percentage' && typeof value === 'number') {
                return `${value.toFixed(1)}%`;
            } else if (type === 'number' && typeof value === 'number') {
                return value.toLocaleString();
            }
            return value || 'N/A';
        },

        getConnectionStatusIcon() {
            const statusIcons = {
                'connected': 'fas fa-circle text-success',
                'connecting': 'fas fa-circle text-warning',
                'reconnecting': 'fas fa-circle text-warning fa-blink',
                'disconnected': 'fas fa-circle text-secondary',
                'error': 'fas fa-circle text-danger'
            };
            return statusIcons[this.connectionStatus] || 'fas fa-circle text-secondary';
        },

        getStatusBadgeClass() {
            const statusClasses = {
                'running': 'bg-success',
                'completed': 'bg-primary',
                'failed': 'bg-danger',
                'pending': 'bg-warning',
                'stopped': 'bg-secondary',
                'monitoring': 'bg-info'
            };
            return statusClasses[this.status] || 'bg-secondary';
        }
    };
}

// Real-time System Monitor Component
function realtimeSystemComponent() {
    return {
        systemMetrics: {},
        isConnected: false,
        lastUpdate: null,
        connectionErrors: 0,

        init() {
            this.connect();
            this.setupEventListeners();
            console.log('Real-time system component initialized');
        },

        setupEventListeners() {
            window.realtimeManager.on('system_connected', () => {
                this.isConnected = true;
                this.connectionErrors = 0;
            });

            window.realtimeManager.on('system_update', (data) => {
                this.handleSystemUpdate(data);
            });

            window.realtimeManager.on('system_error', () => {
                this.isConnected = false;
                this.connectionErrors++;
            });

            window.realtimeManager.on('reconnect_all', () => {
                this.connect();
            });
        },

        connect() {
            window.realtimeManager.connectToSystemUpdates();
        },

        disconnect() {
            window.realtimeManager.disconnectFromSystem();
            this.isConnected = false;
        },

        handleSystemUpdate(data) {
            this.systemMetrics = { ...this.systemMetrics, ...data };
            this.lastUpdate = new Date().toISOString();
        }
    };
}

// Initialize global real-time manager
window.realtimeManager = new RealtimeManager();

// Register components globally
window.realtimeCollectionComponent = realtimeCollectionComponent;
window.realtimeSystemComponent = realtimeSystemComponent;

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
    if (window.realtimeManager) {
        window.realtimeManager.destroy();
    }
});