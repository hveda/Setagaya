package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"
)

// RequestMetrics tracks request metrics
type RequestMetrics struct {
	StartTime    time.Time
	Duration     time.Duration
	StatusCode   int
	RequestSize  int64
	ResponseSize int64
	RequestID    string
}

// LoggingMiddleware provides comprehensive request logging
func (s *SetagayaAPI) LoggingMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		start := time.Now()
		
		// Generate request ID for tracking
		requestID := generateRequestID()
		
		// Create response wrapper to capture metrics
		wrapper := &responseWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			bytesWritten:   0,
		}
		
		// Add request ID to headers
		wrapper.Header().Set("X-Request-ID", requestID)
		
		// Add request ID to context for other middleware/handlers
		r = r.WithContext(addRequestIDToContext(r.Context(), requestID))
		
		// Log request start
		log.WithFields(log.Fields{
			"request_id":     requestID,
			"method":         r.Method,
			"path":           r.URL.Path,
			"query":          r.URL.RawQuery,
			"remote_addr":    r.RemoteAddr,
			"user_agent":     r.UserAgent(),
			"content_length": r.ContentLength,
		}).Info("Request started")
		
		// Process request
		next(wrapper, r, params)
		
		// Calculate metrics
		duration := time.Since(start)
		
		// Log request completion
		logLevel := log.InfoLevel
		if wrapper.statusCode >= 400 {
			logLevel = log.WarnLevel
		}
		if wrapper.statusCode >= 500 {
			logLevel = log.ErrorLevel
		}
		
		log.WithFields(log.Fields{
			"request_id":      requestID,
			"method":          r.Method,
			"path":            r.URL.Path,
			"status_code":     wrapper.statusCode,
			"duration_ms":     duration.Milliseconds(),
			"response_size":   wrapper.bytesWritten,
			"request_size":    r.ContentLength,
		}).Log(logLevel, "Request completed")
		
		// Update metrics (if metrics system is available)
		updateRequestMetrics(&RequestMetrics{
			StartTime:    start,
			Duration:     duration,
			StatusCode:   wrapper.statusCode,
			RequestSize:  r.ContentLength,
			ResponseSize: int64(wrapper.bytesWritten),
			RequestID:    requestID,
		})
	})
}

// MonitoringMiddleware provides performance monitoring
func (s *SetagayaAPI) MonitoringMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		start := time.Now()
		
		// Capture metrics
		wrapper := &responseWrapper{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			bytesWritten:   0,
		}
		
		// Process request
		next(wrapper, r, params)
		
		// Record metrics
		duration := time.Since(start)
		
		// Update Prometheus metrics if available
		updatePrometheusMetrics(r.Method, r.URL.Path, wrapper.statusCode, duration)
		
		// Check for slow requests
		if duration > 5*time.Second {
			log.WithFields(log.Fields{
				"method":      r.Method,
				"path":        r.URL.Path,
				"duration_ms": duration.Milliseconds(),
				"status_code": wrapper.statusCode,
			}).Warn("Slow request detected")
		}
	})
}

// RecoveryMiddleware provides panic recovery
func (s *SetagayaAPI) RecoveryMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		defer func() {
			if err := recover(); err != nil {
				requestID := getRequestIDFromContext(r.Context())
				
				log.WithFields(log.Fields{
					"request_id": requestID,
					"method":     r.Method,
					"path":       r.URL.Path,
					"panic":      fmt.Sprintf("%v", err),
				}).Error("Panic recovered")
				
				// Return 500 error
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "Internal server error", "request_id": "` + requestID + `"}`))
			}
		}()
		
		next(w, r, params)
	})
}

// TimeoutMiddleware provides request timeout handling
func (s *SetagayaAPI) TimeoutMiddleware(timeout time.Duration) func(httprouter.Handle) httprouter.Handle {
	return func(next httprouter.Handle) httprouter.Handle {
		return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
			// Create timeout context
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			
			// Create channel to signal completion
			done := make(chan bool, 1)
			
			// Run handler in goroutine
			go func() {
				next(w, r.WithContext(ctx), params)
				done <- true
			}()
			
			// Wait for completion or timeout
			select {
			case <-done:
				// Request completed normally
				return
			case <-ctx.Done():
				// Request timed out
				requestID := getRequestIDFromContext(r.Context())
				
				log.WithFields(log.Fields{
					"request_id": requestID,
					"method":     r.Method,
					"path":       r.URL.Path,
					"timeout":    timeout.String(),
				}).Warn("Request timeout")
				
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusRequestTimeout)
				w.Write([]byte(`{"error": "Request timeout", "request_id": "` + requestID + `"}`))
			}
		})
	}
}

// CombinedMiddleware combines all monitoring and logging middleware
func (s *SetagayaAPI) CombinedMonitoringMiddleware(next httprouter.Handle) httprouter.Handle {
	return s.RecoveryMiddleware(
		s.LoggingMiddleware(
			s.MonitoringMiddleware(next),
		),
	)
}

// Request ID management

type contextKey string

const requestIDKey contextKey = "request_id"

// Helper types and functions

type responseWrapper struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *responseWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWrapper) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.bytesWritten += n
	return n, err
}

func generateRequestID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func addRequestIDToContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func getRequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return "unknown"
}

// Metrics functions (placeholders for actual implementation)

func updateRequestMetrics(metrics *RequestMetrics) {
	// This would integrate with your metrics system (Prometheus, etc.)
	// For now, it's a placeholder
}

func updatePrometheusMetrics(method, path string, statusCode int, duration time.Duration) {
	// This would update Prometheus metrics
	// For example:
	// requestDuration.WithLabelValues(method, path, strconv.Itoa(statusCode)).Observe(duration.Seconds())
	// requestsTotal.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
}

// HealthCheckMiddleware provides basic health check
func (s *SetagayaAPI) HealthCheckHandler() httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		health := map[string]interface{}{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
			"version":   "1.0.0", // This could be injected at build time
		}
		
		// Add basic system checks
		health["checks"] = map[string]string{
			"database": "ok", // This would check actual DB connection
			"storage":  "ok", // This would check storage connectivity
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		// Simple JSON encoding (in production, use proper JSON marshaling)
		response := fmt.Sprintf(`{
			"status": "%s",
			"timestamp": "%s",
			"version": "%s",
			"checks": {
				"database": "ok",
				"storage": "ok"
			}
		}`, 
			health["status"], 
			health["timestamp"], 
			health["version"],
		)
		w.Write([]byte(response))
	})
}