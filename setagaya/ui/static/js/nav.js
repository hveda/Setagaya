// Navigation Bar Alpine.js component
function topBar() {
    return {
        async logout() {
            try {
                await axios.post('/logout');
                window.location.reload();
            } catch (error) {
                alert('Oops, logout failed...');
            }
        }
    };
}

// Make available globally
window.topBar = topBar;