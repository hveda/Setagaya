package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/julienschmidt/httprouter"
)

const (
	MaxRequestBodySize = 10 << 20 // 10MB
	MaxIDValue         = 999999999
	MaxNameLength      = 255
	MaxDescLength      = 1000
)

var (
	// Common validation patterns
	AlphanumericPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	EmailPattern        = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	ProjectNamePattern  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*[a-zA-Z0-9]$`)
	
	// Error messages
	ErrInvalidRequestBody = errors.New("invalid request body")
	ErrRequestTooLarge    = errors.New("request body too large")
	ErrInvalidID          = errors.New("invalid ID parameter")
	ErrInvalidName        = errors.New("invalid name parameter")
)

// ValidationRule represents a validation rule
type ValidationRule struct {
	Field     string
	Required  bool
	MinLength int
	MaxLength int
	Pattern   *regexp.Regexp
	Validator func(interface{}) error
}

// RequestValidator handles request validation
type RequestValidator struct {
	Rules map[string][]ValidationRule
}

// NewRequestValidator creates a new request validator
func NewRequestValidator() *RequestValidator {
	return &RequestValidator{
		Rules: make(map[string][]ValidationRule),
	}
}

// AddRule adds a validation rule
func (rv *RequestValidator) AddRule(endpoint string, rule ValidationRule) {
	if rv.Rules[endpoint] == nil {
		rv.Rules[endpoint] = []ValidationRule{}
	}
	rv.Rules[endpoint] = append(rv.Rules[endpoint], rule)
}

// InputValidationMiddleware provides input validation for API endpoints
func (s *ShibuyaAPI) InputValidationMiddleware(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		// Validate request size
		if err := validateRequestSize(r); err != nil {
			s.handleErrors(w, makeInvalidRequestError(err.Error()))
			return
		}
		
		// Validate path parameters
		if err := validatePathParameters(params); err != nil {
			s.handleErrors(w, makeInvalidRequestError(err.Error()))
			return
		}
		
		// Validate query parameters
		if err := validateQueryParameters(r); err != nil {
			s.handleErrors(w, makeInvalidRequestError(err.Error()))
			return
		}
		
		// Validate request body if present
		if r.ContentLength > 0 {
			if err := s.validateRequestBody(r); err != nil {
				s.handleErrors(w, makeInvalidRequestError(err.Error()))
				return
			}
		}
		
		// Continue to next handler
		next(w, r, params)
	})
}

// validateRequestSize validates the request size
func validateRequestSize(r *http.Request) error {
	if r.ContentLength > MaxRequestBodySize {
		return ErrRequestTooLarge
	}
	
	// Limit body reader to prevent memory exhaustion
	r.Body = http.MaxBytesReader(nil, r.Body, MaxRequestBodySize)
	return nil
}

// validatePathParameters validates URL path parameters
func validatePathParameters(params httprouter.Params) error {
	for _, param := range params {
		switch param.Key {
		case "project_id", "collection_id", "plan_id":
			if err := validateID(param.Value); err != nil {
				return fmt.Errorf("invalid %s: %w", param.Key, err)
			}
		case "filename":
			if err := validateFilename(param.Value); err != nil {
				return fmt.Errorf("invalid filename: %w", err)
			}
		default:
			// Generic validation for unknown parameters
			if err := validateGenericParam(param.Value); err != nil {
				return fmt.Errorf("invalid parameter %s: %w", param.Key, err)
			}
		}
	}
	return nil
}

// validateQueryParameters validates URL query parameters
func validateQueryParameters(r *http.Request) error {
	query := r.URL.Query()
	
	for key, values := range query {
		for _, value := range values {
			switch key {
			case "limit", "offset", "page":
				if err := validateNumericParam(value); err != nil {
					return fmt.Errorf("invalid %s parameter: %w", key, err)
				}
			case "sort", "order":
				if err := validateSortParam(value); err != nil {
					return fmt.Errorf("invalid %s parameter: %w", key, err)
				}
			case "filter":
				if err := validateFilterParam(value); err != nil {
					return fmt.Errorf("invalid filter parameter: %w", err)
				}
			default:
				// Generic validation for unknown query parameters
				if err := validateGenericParam(value); err != nil {
					return fmt.Errorf("invalid query parameter %s: %w", key, err)
				}
			}
		}
	}
	return nil
}

// validateRequestBody validates JSON request body
func (s *ShibuyaAPI) validateRequestBody(r *http.Request) error {
	// Read and validate JSON structure
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	
	// Reset body for further processing
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	
	// Validate JSON syntax
	var jsonData interface{}
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	
	// Validate JSON content based on endpoint
	return s.validateJSONContent(r.URL.Path, jsonData)
}

// validateJSONContent validates JSON content based on endpoint
func (s *ShibuyaAPI) validateJSONContent(path string, data interface{}) error {
	switch {
	case strings.Contains(path, "/projects"):
		return validateProjectJSON(data)
	case strings.Contains(path, "/collections"):
		return validateCollectionJSON(data)
	case strings.Contains(path, "/plans"):
		return validatePlanJSON(data)
	default:
		return validateGenericJSON(data)
	}
}

// Specific validation functions

func validateID(value string) error {
	if value == "" {
		return ErrInvalidID
	}
	
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return ErrInvalidID
	}
	
	if id <= 0 || id > MaxIDValue {
		return ErrInvalidID
	}
	
	return nil
}

func validateFilename(filename string) error {
	if filename == "" {
		return errors.New("filename cannot be empty")
	}
	
	if len(filename) > MaxNameLength {
		return errors.New("filename too long")
	}
	
	// Check for path traversal attempts
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return errors.New("invalid filename: path traversal detected")
	}
	
	// Check for null bytes
	if strings.Contains(filename, "\x00") {
		return errors.New("invalid filename: null bytes not allowed")
	}
	
	// Validate filename characters
	for _, r := range filename {
		if !unicode.IsPrint(r) {
			return errors.New("invalid filename: non-printable characters not allowed")
		}
	}
	
	return nil
}

func validateGenericParam(value string) error {
	if len(value) > 1000 { // Reasonable limit for generic parameters
		return errors.New("parameter value too long")
	}
	
	// Check for null bytes and control characters
	for _, r := range value {
		if r == 0 || (r < 32 && r != 9 && r != 10 && r != 13) {
			return errors.New("invalid characters in parameter")
		}
	}
	
	return nil
}

func validateNumericParam(value string) error {
	if value == "" {
		return errors.New("numeric parameter cannot be empty")
	}
	
	num, err := strconv.Atoi(value)
	if err != nil {
		return errors.New("invalid numeric parameter")
	}
	
	if num < 0 || num > 10000 { // Reasonable limits
		return errors.New("numeric parameter out of range")
	}
	
	return nil
}

func validateSortParam(value string) error {
	allowedValues := []string{"asc", "desc", "created_at", "updated_at", "name", "id"}
	
	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}
	
	return errors.New("invalid sort parameter")
}

func validateFilterParam(value string) error {
	// Basic filter validation - prevent injection attacks
	if len(value) > 500 {
		return errors.New("filter parameter too long")
	}
	
	// Check for SQL injection patterns
	dangerousPatterns := []string{
		"'", "\"", ";", "--", "/*", "*/", "union", "select", "insert", "update", "delete", "drop",
	}
	
	lowerValue := strings.ToLower(value)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerValue, pattern) {
			return fmt.Errorf("filter parameter contains dangerous pattern: %s", pattern)
		}
	}
	
	return nil
}

// JSON validation functions

func validateProjectJSON(data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return errors.New("project data must be a JSON object")
	}
	
	// Validate required fields
	if name, exists := dataMap["name"]; exists {
		nameStr, ok := name.(string)
		if !ok {
			return errors.New("project name must be a string")
		}
		
		if err := validateProjectName(nameStr); err != nil {
			return fmt.Errorf("invalid project name: %w", err)
		}
	}
	
	return nil
}

func validateCollectionJSON(data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return errors.New("collection data must be a JSON object")
	}
	
	// Validate collection-specific fields
	if tests, exists := dataMap["tests"]; exists {
		testsArray, ok := tests.([]interface{})
		if !ok {
			return errors.New("tests field must be an array")
		}
		
		if len(testsArray) > 100 { // Reasonable limit
			return errors.New("too many tests in collection")
		}
	}
	
	return nil
}

func validatePlanJSON(data interface{}) error {
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return errors.New("plan data must be a JSON object")
	}
	
	// Validate plan-specific fields
	if engines, exists := dataMap["engines"]; exists {
		enginesNum, ok := engines.(float64)
		if !ok {
			return errors.New("engines field must be a number")
		}
		
		if enginesNum <= 0 || enginesNum > 1000 {
			return errors.New("engines count out of range")
		}
	}
	
	return nil
}

func validateGenericJSON(data interface{}) error {
	// Perform basic validation for any JSON
	switch v := data.(type) {
	case map[string]interface{}:
		// Limit number of keys to prevent DoS
		if len(v) > 1000 {
			return errors.New("JSON object has too many keys")
		}
		
		// Recursively validate nested objects
		for key, value := range v {
			if len(key) > MaxNameLength {
				return errors.New("JSON key too long")
			}
			
			if err := validateGenericJSON(value); err != nil {
				return err
			}
		}
		
	case []interface{}:
		// Limit array size
		if len(v) > 10000 {
			return errors.New("JSON array too large")
		}
		
		// Recursively validate array elements
		for _, item := range v {
			if err := validateGenericJSON(item); err != nil {
				return err
			}
		}
		
	case string:
		// Validate string length and content
		if len(v) > 10000 {
			return errors.New("JSON string too long")
		}
		
		// Check for null bytes
		if strings.Contains(v, "\x00") {
			return errors.New("JSON string contains null bytes")
		}
	}
	
	return nil
}

func validateProjectName(name string) error {
	if name == "" {
		return errors.New("project name cannot be empty")
	}
	
	if len(name) > MaxNameLength {
		return errors.New("project name too long")
	}
	
	if !ProjectNamePattern.MatchString(name) {
		return errors.New("project name contains invalid characters")
	}
	
	return nil
}