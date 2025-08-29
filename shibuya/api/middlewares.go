package api

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/rakutentech/shibuya/shibuya/auth"
	"github.com/rakutentech/shibuya/shibuya/model"
)

const (
	accountKey = "account"
)

// getAccountFromSession creates an Account from legacy session
func getAccountFromSession(r *http.Request) *auth.Account {
	legacyAccount := model.GetAccountBySession(r)
	if legacyAccount == nil {
		return nil
	}
	
	// Convert legacy account to new account structure
	return &auth.Account{
		Name:   legacyAccount.Name,
		ML:     legacyAccount.ML,
		MLMap:  legacyAccount.MLMap,
		Groups: legacyAccount.ML, // Use ML as groups for compatibility
	}
}

// authWithSession authenticates using secure session management
func authWithSession(r *http.Request) (*auth.Account, error) {
	if auth.DefaultSessionManager == nil {
		// Fallback to legacy session management
		account := getAccountFromSession(r)
		if account == nil {
			return nil, makeLoginError()
		}
		return account, nil
	}
	
	// Use secure session management
	sessionData, err := auth.DefaultSessionManager.GetSession(r)
	if err != nil {
		return nil, makeLoginError()
	}
	
	// Create account from session data
	account := &auth.Account{
		Name:   sessionData.Username,
		Groups: sessionData.Groups,
		ML:     sessionData.Groups, // Use groups as ML for compatibility
	}
	
	return account, nil
}

// authWithToken authenticates using JWT token
func authWithToken(r *http.Request) (*auth.Account, error) {
	return auth.AuthWithJWT(r)
}

// authRequired middleware with enhanced security
func (s *ShibuyaAPI) authRequired(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		var account *auth.Account
		var err error
		
		// Try JWT authentication first
		account, err = authWithToken(r)
		if err != nil {
			// Fall back to session authentication
			account, err = authWithSession(r)
			if err != nil {
				s.handleErrors(w, err)
				return
			}
		}
		
		// Add account to request context
		next(w, r.WithContext(context.WithValue(r.Context(), accountKey, account)), params)
	})
}

// SecurityHeadersMiddleware adds security headers to all responses
func (s *ShibuyaAPI) SecurityHeadersMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		
		// CORS headers for API endpoints
		if r.URL.Path[:4] == "/api" {
			w.Header().Set("Access-Control-Allow-Origin", "*") // Configure appropriately for production
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}
		
		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		
		next(w, r, params)
	})
}

// CSRFMiddleware validates CSRF tokens for state-changing operations
func (s *ShibuyaAPI) CSRFMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		// Only check CSRF for state-changing methods
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" {
			// Get CSRF token from header or form
			csrfToken := r.Header.Get("X-CSRF-Token")
			if csrfToken == "" {
				csrfToken = r.FormValue("csrf_token")
			}
			
			// Validate CSRF token if session management is enabled
			if auth.DefaultSessionManager != nil {
				if !auth.DefaultSessionManager.ValidateCSRFToken(r, csrfToken) {
					s.handleErrors(w, makeInvalidRequestError("Invalid CSRF token"))
					return
				}
			}
		}
		
		next(w, r, params)
	})
}

// CombinedSecurityMiddleware combines all security middlewares
func (s *ShibuyaAPI) CombinedSecurityMiddleware(next httprouter.Handle) httprouter.Handle {
	return s.SecurityHeadersMiddleware(
		s.RateLimitMiddleware(
			s.InputValidationMiddleware(
				s.CSRFMiddleware(next),
			),
		),
	)
}

// SecureAuthRequired combines authentication with security middleware
func (s *ShibuyaAPI) SecureAuthRequired(next httprouter.Handle) httprouter.Handle {
	return s.CombinedSecurityMiddleware(s.authRequired(next))
}
