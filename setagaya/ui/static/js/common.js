// Setagaya Common Utilities - Alpine.js Version
// Converted from Vue.js to Alpine.js for Phase 2

// Global constants
window.SYNC_INTERVAL = 5000;

// Configure axios defaults
axios.defaults.baseURL = '/api';
axios.defaults.headers.post['Content-Type'] = 'application/json';

// Common error handling function
function handleErrorResponse(error) {
    if (error.response && error.response.data && error.response.data.message) {
        return error.response.data.message;
    } else if (error.response && error.response.data && typeof error.response.data === 'string') {
        return error.response.data;
    } else if (error.response && error.response.statusText) {
        return error.response.statusText;
    } else if (error.message) {
        return error.message;
    } else {
        return 'An error occurred';
    }
}

// Time zone formatting helper
function toLocalTZ(isodate) {
    if (!isodate) return 'N/A';
    
    const d = new Date(isodate);
    if (d <= Date.UTC(1970)) {
        return "Running";
    }
    
    return Intl.DateTimeFormat('en-jp', {
        year: 'numeric', 
        month: 'short', 
        day: 'numeric', 
        hour: 'numeric',
        minute: 'numeric', 
        second: '2-digit', 
        timeZoneName: 'short'
    }).format(d);
}

// File upload helper function
async function uploadFile(file, url, inputName = 'file') {
    const formData = new FormData();
    formData.append(inputName, file, file.name);
    
    try {
        const response = await axios.put(`/api/${url}`, formData, {
            headers: {
                'Content-Type': 'multipart/form-data'
            }
        });
        
        if (response.status === 200) {
            alert("Upload success!");
            return true;
        }
    } catch (error) {
        alert('Upload failed: ' + handleErrorResponse(error));
        return false;
    }
}

// Generic form submission helper
async function submitForm(url, payload) {
    try {
        const response = await axios.post(url, payload);
        return response.data;
    } catch (error) {
        throw new Error(handleErrorResponse(error));
    }
}

// Make functions globally available
window.handleErrorResponse = handleErrorResponse;
window.toLocalTZ = toLocalTZ;
window.uploadFile = uploadFile;
window.submitForm = submitForm;
