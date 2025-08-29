package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute   int
	BurstSize          int
	AuthRequestsPerMin int
	AuthBurstSize      int
	CleanupInterval    time.Duration
}

// ClientInfo tracks rate limiting info for a client
type ClientInfo struct {
	Requests    int
	LastReset   time.Time
	Blocked     bool
	BlockedUntil time.Time
}

// RateLimiter manages rate limiting for API endpoints
type RateLimiter struct {
	clients     map[string]*ClientInfo
	mutex       sync.RWMutex
	config      RateLimitConfig
	stopCleanup chan bool
}

var (
	DefaultRateLimiter *RateLimiter
)

// InitRateLimiter initializes the default rate limiter
func InitRateLimiter() {
	config := RateLimitConfig{
		RequestsPerMinute:   100, // General API requests
		BurstSize:          20,   // Burst allowance
		AuthRequestsPerMin:  10,  // Authentication requests (more restrictive)
		AuthBurstSize:      3,    // Small burst for auth
		CleanupInterval:    5 * time.Minute,
	}
	
	DefaultRateLimiter = NewRateLimiter(config)
	DefaultRateLimiter.Start()
	
	log.Info("Rate limiter initialized")
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		clients:     make(map[string]*ClientInfo),
		config:      config,
		stopCleanup: make(chan bool),
	}
}

// Start begins the cleanup routine
func (rl *RateLimiter) Start() {
	go rl.cleanupRoutine()
}

// Stop stops the cleanup routine
func (rl *RateLimiter) Stop() {
	close(rl.stopCleanup)
}

// RateLimitMiddleware provides rate limiting for API endpoints
func (s *ShibuyaAPI) RateLimitMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		// Extract client identifier
		clientID := extractClientID(r)
		
		// Determine if this is an auth endpoint
		isAuth := isAuthEndpoint(r.URL.Path)
		
		// Check rate limit
		allowed, retryAfter := DefaultRateLimiter.Allow(clientID, isAuth)
		if !allowed {
			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", "100")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", retryAfter.Format(time.RFC3339))
			w.Header().Set("Retry-After", retryAfter.Format(time.RFC3339))
			
			// Log rate limit violation
			log.WithFields(log.Fields{
				"client_id": clientID,
				"endpoint":  r.URL.Path,
				"method":    r.Method,
				"is_auth":   isAuth,
			}).Warn("Rate limit exceeded")
			
			s.handleErrors(w, makeInvalidRequestError("Rate limit exceeded"))
			return
		}
		
		// Set rate limit headers for successful requests
		DefaultRateLimiter.setRateLimitHeaders(w, clientID, isAuth)
		
		// Continue to next handler
		next(w, r, params)
	})
}

// Allow checks if a client is allowed to make a request
func (rl *RateLimiter) Allow(clientID string, isAuth bool) (bool, time.Time) {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	now := time.Now()
	
	// Get or create client info
	client, exists := rl.clients[clientID]
	if !exists {
		client = &ClientInfo{
			Requests:  0,
			LastReset: now,
		}
		rl.clients[clientID] = client
	}
	
	// Check if client is currently blocked
	if client.Blocked && now.Before(client.BlockedUntil) {
		return false, client.BlockedUntil
	}
	
	// Reset blocked status if block period has expired
	if client.Blocked && now.After(client.BlockedUntil) {
		client.Blocked = false
		client.Requests = 0
		client.LastReset = now
	}
	
	// Reset counter if more than a minute has passed
	if now.Sub(client.LastReset) >= time.Minute {
		client.Requests = 0
		client.LastReset = now
	}
	
	// Determine limits based on endpoint type
	var limit, burst int
	if isAuth {
		limit = rl.config.AuthRequestsPerMin
		burst = rl.config.AuthBurstSize
	} else {
		limit = rl.config.RequestsPerMinute
		burst = rl.config.BurstSize
	}
	
	// Check if request is within limits
	if client.Requests >= limit {
		// Block client for progressive penalty
		blockDuration := rl.calculateBlockDuration(client.Requests, limit)
		client.Blocked = true
		client.BlockedUntil = now.Add(blockDuration)
		
		log.WithFields(log.Fields{
			"client_id":      clientID,
			"requests":       client.Requests,
			"limit":          limit,
			"block_duration": blockDuration,
		}).Warn("Client blocked due to rate limit violation")
		
		return false, client.BlockedUntil
	}
	
	// Allow burst if within burst limit or if requests are under normal limit
	if client.Requests < burst || client.Requests < limit {
		client.Requests++
		return true, time.Time{}
	}
	
	return false, now.Add(time.Minute)
}

// calculateBlockDuration calculates progressive block duration
func (rl *RateLimiter) calculateBlockDuration(requests, limit int) time.Duration {
	// Progressive blocking: more violations = longer blocks
	violations := requests - limit
	
	switch {
	case violations <= 5:
		return 1 * time.Minute
	case violations <= 10:
		return 5 * time.Minute
	case violations <= 20:
		return 15 * time.Minute
	default:
		return 1 * time.Hour // Maximum block duration
	}
}

// setRateLimitHeaders sets rate limit headers for responses
func (rl *RateLimiter) setRateLimitHeaders(w http.ResponseWriter, clientID string, isAuth bool) {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()
	
	client, exists := rl.clients[clientID]
	if !exists {
		return
	}
	
	// Determine limits
	var limit int
	if isAuth {
		limit = rl.config.AuthRequestsPerMin
	} else {
		limit = rl.config.RequestsPerMinute
	}
	
	remaining := limit - client.Requests
	if remaining < 0 {
		remaining = 0
	}
	
	reset := client.LastReset.Add(time.Minute)
	
	w.Header().Set("X-RateLimit-Limit", string(rune(limit)))
	w.Header().Set("X-RateLimit-Remaining", string(rune(remaining)))
	w.Header().Set("X-RateLimit-Reset", reset.Format(time.RFC3339))
}

// cleanupRoutine periodically cleans up old client entries
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.config.CleanupInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCleanup:
			return
		}
	}
}

// cleanup removes old client entries to prevent memory leaks
func (rl *RateLimiter) cleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-10 * time.Minute) // Remove entries older than 10 minutes
	
	for clientID, client := range rl.clients {
		if client.LastReset.Before(cutoff) && !client.Blocked {
			delete(rl.clients, clientID)
		}
	}
	
	log.WithField("remaining_clients", len(rl.clients)).Debug("Rate limiter cleanup completed")
}

// Helper functions

// extractClientID extracts a unique identifier for the client
func extractClientID(r *http.Request) string {
	// Try to get real IP from headers (proxy-aware)
	if xRealIP := r.Header.Get("X-Real-IP"); xRealIP != "" {
		return xRealIP
	}
	
	if xForwardedFor := r.Header.Get("X-Forwarded-For"); xForwardedFor != "" {
		// Take the first IP in the chain
		return strings.Split(xForwardedFor, ",")[0]
	}
	
	// Fall back to remote address
	return r.RemoteAddr
}

// isAuthEndpoint determines if an endpoint is authentication-related
func isAuthEndpoint(path string) bool {
	authPaths := []string{
		"/login",
		"/logout", 
		"/auth",
		"/token",
		"/refresh",
	}
	
	for _, authPath := range authPaths {
		if strings.Contains(path, authPath) {
			return true
		}
	}
	
	return false
}

// GetRateLimitStats returns current rate limiting statistics
func (rl *RateLimiter) GetRateLimitStats() map[string]interface{} {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()
	
	stats := map[string]interface{}{
		"total_clients": len(rl.clients),
		"blocked_clients": 0,
		"config": rl.config,
	}
	
	blockedCount := 0
	for _, client := range rl.clients {
		if client.Blocked {
			blockedCount++
		}
	}
	
	stats["blocked_clients"] = blockedCount
	return stats
}