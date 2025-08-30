// Setagaya Enhanced File Upload System - Phase 3
// Drag & Drop File Upload with Progress Indicators and Advanced Features

// File Upload Component
function fileUploadComponent() {
    return {
        files: [],
        uploadQueue: [],
        isDragOver: false,
        isUploading: false,
        uploadProgress: {},
        allowedTypes: ['.jmx', '.jar', '.csv', '.properties', '.txt'],
        maxFileSize: 50 * 1024 * 1024, // 50MB
        errors: [],
        successMessages: [],

        init() {
            console.log('File upload component initialized');
            this.setupDropHandlers();
        },

        setupDropHandlers() {
            // Setup drag and drop handlers for the component
            this.$el.addEventListener('dragover', (e) => {
                e.preventDefault();
                this.isDragOver = true;
            });

            this.$el.addEventListener('dragleave', (e) => {
                e.preventDefault();
                if (!this.$el.contains(e.relatedTarget)) {
                    this.isDragOver = false;
                }
            });

            this.$el.addEventListener('drop', (e) => {
                e.preventDefault();
                this.isDragOver = false;
                this.handleFiles(e.dataTransfer.files);
            });
        },

        handleFiles(fileList) {
            this.errors = [];
            
            Array.from(fileList).forEach(file => {
                if (this.validateFile(file)) {
                    this.addFileToQueue(file);
                }
            });
        },

        validateFile(file) {
            // Check file size
            if (file.size > this.maxFileSize) {
                this.addError(`File ${file.name} is too large. Maximum size is ${this.formatFileSize(this.maxFileSize)}`);
                return false;
            }

            // Check file type
            const fileExtension = '.' + file.name.split('.').pop().toLowerCase();
            if (!this.allowedTypes.includes(fileExtension)) {
                this.addError(`File ${file.name} has an unsupported format. Allowed types: ${this.allowedTypes.join(', ')}`);
                return false;
            }

            // Check for duplicates
            if (this.files.some(f => f.name === file.name && f.size === file.size)) {
                this.addError(`File ${file.name} is already selected`);
                return false;
            }

            return true;
        },

        addFileToQueue(file) {
            const fileData = {
                id: this.generateFileId(),
                file: file,
                name: file.name,
                size: file.size,
                type: file.type,
                status: 'pending',
                progress: 0,
                error: null,
                preview: null
            };

            this.files.push(fileData);
            this.generateFilePreview(fileData);
        },

        async generateFilePreview(fileData) {
            if (fileData.file.type.startsWith('text/') || fileData.name.endsWith('.jmx')) {
                try {
                    const reader = new FileReader();
                    reader.onload = (e) => {
                        fileData.preview = e.target.result.substring(0, 500) + (e.target.result.length > 500 ? '...' : '');
                    };
                    reader.readAsText(fileData.file);
                } catch (error) {
                    console.log('Could not generate preview for', fileData.name);
                }
            }
        },

        async uploadFiles() {
            if (this.files.length === 0) {
                this.addError('No files selected');
                return;
            }

            if (!window.authManager.hasPermission('files:upload')) {
                this.addError('You do not have permission to upload files');
                return;
            }

            this.isUploading = true;
            this.errors = [];
            this.successMessages = [];

            const pendingFiles = this.files.filter(f => f.status === 'pending');
            
            for (const fileData of pendingFiles) {
                await this.uploadSingleFile(fileData);
            }

            this.isUploading = false;
        },

        async uploadSingleFile(fileData) {
            fileData.status = 'uploading';
            fileData.progress = 0;

            const formData = new FormData();
            formData.append('file', fileData.file);

            try {
                const response = await axios.post('/api/files/upload', formData, {
                    headers: {
                        'Content-Type': 'multipart/form-data'
                    },
                    onUploadProgress: (progressEvent) => {
                        fileData.progress = Math.round((progressEvent.loaded * 100) / progressEvent.total);
                    }
                });

                fileData.status = 'completed';
                fileData.progress = 100;
                this.addSuccess(`Successfully uploaded ${fileData.name}`);

                // Emit event for parent components
                this.$dispatch('file-uploaded', {
                    file: fileData,
                    response: response.data
                });

            } catch (error) {
                fileData.status = 'error';
                fileData.error = error.response?.data?.message || 'Upload failed';
                this.addError(`Failed to upload ${fileData.name}: ${fileData.error}`);
            }
        },

        removeFile(fileId) {
            const index = this.files.findIndex(f => f.id === fileId);
            if (index !== -1) {
                this.files.splice(index, 1);
            }
        },

        clearAll() {
            this.files = [];
            this.errors = [];
            this.successMessages = [];
        },

        retryUpload(fileId) {
            const file = this.files.find(f => f.id === fileId);
            if (file) {
                file.status = 'pending';
                file.progress = 0;
                file.error = null;
            }
        },

        // Utility functions
        generateFileId() {
            return Date.now().toString(36) + Math.random().toString(36).substr(2);
        },

        formatFileSize(bytes) {
            if (bytes === 0) return '0 Bytes';
            const k = 1024;
            const sizes = ['Bytes', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        },

        getFileIcon(fileName) {
            const extension = fileName.split('.').pop().toLowerCase();
            const iconMap = {
                'jmx': 'fas fa-cogs',
                'jar': 'fas fa-archive',
                'csv': 'fas fa-table',
                'properties': 'fas fa-cog',
                'txt': 'fas fa-file-alt'
            };
            return iconMap[extension] || 'fas fa-file';
        },

        getStatusIcon(status) {
            const statusMap = {
                'pending': 'fas fa-clock text-warning',
                'uploading': 'fas fa-spinner fa-spin text-primary',
                'completed': 'fas fa-check-circle text-success',
                'error': 'fas fa-exclamation-circle text-danger'
            };
            return statusMap[status] || 'fas fa-file';
        },

        getStatusText(status) {
            const statusMap = {
                'pending': 'Pending',
                'uploading': 'Uploading...',
                'completed': 'Completed',
                'error': 'Error'
            };
            return statusMap[status] || 'Unknown';
        },

        addError(message) {
            this.errors.push(message);
        },

        addSuccess(message) {
            this.successMessages.push(message);
        },

        clearMessages() {
            this.errors = [];
            this.successMessages = [];
        }
    }
}

// File Manager Component for browsing uploaded files
function fileManagerComponent() {
    return {
        files: [],
        loading: false,
        error: null,
        selectedFiles: new Set(),
        sortBy: 'name',
        sortDirection: 'asc',
        searchTerm: '',
        currentPage: 1,
        itemsPerPage: 20,

        async init() {
            await this.loadFiles();
            console.log('File manager initialized');
        },

        async loadFiles() {
            if (!window.authManager.hasPermission('files:read')) {
                this.error = 'You do not have permission to view files';
                return;
            }

            try {
                this.loading = true;
                const response = await axios.get('/api/files');
                this.files = response.data.files || [];
                this.error = null;
            } catch (error) {
                console.error('Failed to load files:', error);
                this.error = 'Failed to load files';
            } finally {
                this.loading = false;
            }
        },

        get filteredFiles() {
            let filtered = this.files;

            // Apply search filter
            if (this.searchTerm) {
                filtered = filtered.filter(file => 
                    file.name.toLowerCase().includes(this.searchTerm.toLowerCase())
                );
            }

            // Apply sorting
            filtered.sort((a, b) => {
                let aVal = a[this.sortBy];
                let bVal = b[this.sortBy];

                if (this.sortBy === 'size') {
                    aVal = parseInt(aVal) || 0;
                    bVal = parseInt(bVal) || 0;
                } else if (this.sortBy === 'uploaded_at') {
                    aVal = new Date(aVal);
                    bVal = new Date(bVal);
                }

                if (aVal < bVal) return this.sortDirection === 'asc' ? -1 : 1;
                if (aVal > bVal) return this.sortDirection === 'asc' ? 1 : -1;
                return 0;
            });

            return filtered;
        },

        get paginatedFiles() {
            const start = (this.currentPage - 1) * this.itemsPerPage;
            const end = start + this.itemsPerPage;
            return this.filteredFiles.slice(start, end);
        },

        get totalPages() {
            return Math.ceil(this.filteredFiles.length / this.itemsPerPage);
        },

        setSorting(field) {
            if (this.sortBy === field) {
                this.sortDirection = this.sortDirection === 'asc' ? 'desc' : 'asc';
            } else {
                this.sortBy = field;
                this.sortDirection = 'asc';
            }
        },

        toggleFileSelection(fileId) {
            if (this.selectedFiles.has(fileId)) {
                this.selectedFiles.delete(fileId);
            } else {
                this.selectedFiles.add(fileId);
            }
        },

        selectAllFiles() {
            this.paginatedFiles.forEach(file => {
                this.selectedFiles.add(file.id);
            });
        },

        clearSelection() {
            this.selectedFiles.clear();
        },

        async deleteFile(fileId) {
            if (!window.authManager.hasPermission('files:delete')) {
                alert('You do not have permission to delete files');
                return;
            }

            if (!confirm('Are you sure you want to delete this file?')) {
                return;
            }

            try {
                await axios.delete(`/api/files/${fileId}`);
                await this.loadFiles();
                this.selectedFiles.delete(fileId);
            } catch (error) {
                console.error('Failed to delete file:', error);
                alert('Failed to delete file');
            }
        },

        async deleteSelectedFiles() {
            if (this.selectedFiles.size === 0) {
                alert('No files selected');
                return;
            }

            if (!confirm(`Are you sure you want to delete ${this.selectedFiles.size} selected files?`)) {
                return;
            }

            for (const fileId of this.selectedFiles) {
                await this.deleteFile(fileId);
            }
        },

        downloadFile(fileId, fileName) {
            if (!window.authManager.hasPermission('files:download')) {
                alert('You do not have permission to download files');
                return;
            }

            const link = document.createElement('a');
            link.href = `/api/files/${fileId}/download`;
            link.download = fileName;
            link.click();
        },

        formatFileSize(bytes) {
            if (bytes === 0) return '0 Bytes';
            const k = 1024;
            const sizes = ['Bytes', 'KB', 'MB', 'GB'];
            const i = Math.floor(Math.log(bytes) / Math.log(k));
            return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
        },

        formatDate(dateString) {
            return dateString ? new Date(dateString).toLocaleString() : 'N/A';
        },

        getFileIcon(fileName) {
            const extension = fileName.split('.').pop().toLowerCase();
            const iconMap = {
                'jmx': 'fas fa-cogs text-primary',
                'jar': 'fas fa-archive text-warning',
                'csv': 'fas fa-table text-success',
                'properties': 'fas fa-cog text-info',
                'txt': 'fas fa-file-alt text-secondary'
            };
            return iconMap[extension] || 'fas fa-file';
        }
    }
}

// File Preview Component
function filePreviewComponent() {
    return {
        file: null,
        content: '',
        loading: false,
        error: null,
        previewType: 'text',

        async showPreview(fileId) {
            if (!window.authManager.hasPermission('files:read')) {
                this.error = 'You do not have permission to view files';
                return;
            }

            try {
                this.loading = true;
                this.error = null;

                const response = await axios.get(`/api/files/${fileId}`);
                this.file = response.data.file;

                if (this.canPreviewAsText()) {
                    await this.loadTextContent();
                } else {
                    this.previewType = 'binary';
                }

            } catch (error) {
                console.error('Failed to load file preview:', error);
                this.error = 'Failed to load file preview';
            } finally {
                this.loading = false;
            }
        },

        async loadTextContent() {
            try {
                const response = await axios.get(`/api/files/${this.file.id}/content`, {
                    responseType: 'text'
                });
                this.content = response.data;
                this.previewType = 'text';
            } catch (error) {
                this.error = 'Could not load file content';
                this.previewType = 'binary';
            }
        },

        canPreviewAsText() {
            if (!this.file) return false;
            
            const textExtensions = ['.jmx', '.txt', '.properties', '.csv', '.xml', '.json'];
            const extension = '.' + this.file.name.split('.').pop().toLowerCase();
            
            return textExtensions.includes(extension) && this.file.size < 1024 * 1024; // Max 1MB for text preview
        },

        closePreview() {
            this.file = null;
            this.content = '';
            this.error = null;
            this.previewType = 'text';
        }
    }
}

// Register components globally
window.fileUploadComponent = fileUploadComponent;
window.fileManagerComponent = fileManagerComponent;
window.filePreviewComponent = filePreviewComponent;