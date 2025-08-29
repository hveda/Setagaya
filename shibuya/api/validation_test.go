package api

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/assert"
)

func TestValidateID(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		shouldErr bool
	}{
		{"valid ID", "123", false},
		{"valid large ID", "999999", false},
		{"empty string", "", true},
		{"negative ID", "-1", true},
		{"zero ID", "0", true},
		{"non-numeric", "abc", true},
		{"too large", "1000000000", true},
		{"float", "123.45", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateID(tt.value)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		shouldErr bool
	}{
		{"valid filename", "test.txt", false},
		{"valid with numbers", "test123.xml", false},
		{"valid with dash", "test-file.jmx", false},
		{"valid with underscore", "test_file.csv", false},
		{"empty filename", "", true},
		{"path traversal", "../test.txt", true},
		{"forward slash", "path/test.txt", true},
		{"backslash", "path\\test.txt", true},
		{"null byte", "test\x00.txt", true},
		{"too long", strings.Repeat("a", 300), true},
		{"non-printable", "test\x01.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilename(tt.filename)
			if tt.shouldErr {
				assert.Error(t, err, "Expected error for filename: %s", tt.filename)
			} else {
				assert.NoError(t, err, "Expected no error for filename: %s", tt.filename)
			}
		})
	}
}

func TestValidateGenericParam(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		shouldErr bool
	}{
		{"normal text", "hello world", false},
		{"with tab", "hello\tworld", false},
		{"with newline", "hello\nworld", false},
		{"with carriage return", "hello\rworld", false},
		{"too long", strings.Repeat("a", 1001), true},
		{"null byte", "hello\x00world", true},
		{"control character", "hello\x01world", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGenericParam(tt.value)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNumericParam(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		shouldErr bool
	}{
		{"valid number", "123", false},
		{"zero", "0", false},
		{"max allowed", "10000", false},
		{"empty string", "", true},
		{"negative", "-1", true},
		{"too large", "10001", true},
		{"non-numeric", "abc", true},
		{"float", "123.45", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNumericParam(tt.value)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSortParam(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		shouldErr bool
	}{
		{"ascending", "asc", false},
		{"descending", "desc", false},
		{"created_at", "created_at", false},
		{"updated_at", "updated_at", false},
		{"name", "name", false},
		{"id", "id", false},
		{"invalid", "invalid_sort", true},
		{"empty", "", true},
		{"sql injection", "'; DROP TABLE", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSortParam(tt.value)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFilterParam(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		shouldErr bool
	}{
		{"simple filter", "status=active", false},
		{"complex filter", "name=test AND status=active", false},
		{"empty", "", false},
		{"too long", strings.Repeat("a", 501), true},
		{"sql injection - quote", "'; DROP TABLE users; --", true},
		{"sql injection - union", "1 UNION SELECT * FROM users", true},
		{"sql injection - comment", "1 /* comment */ OR 1=1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilterParam(tt.value)
			if tt.shouldErr {
				assert.Error(t, err, "Expected error for filter: %s", tt.value)
			} else {
				assert.NoError(t, err, "Expected no error for filter: %s", tt.value)
			}
		})
	}
}

func TestValidateProjectName(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		shouldErr bool
	}{
		{"valid name", "myproject", false},
		{"with numbers", "project123", false},
		{"with dash", "my-project", false},
		{"with underscore", "my_project", false},
		{"start end with alphanumeric", "p1", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 256), true},
		{"start with dash", "-project", true},
		{"end with dash", "project-", true},
		{"start with underscore", "_project", true},
		{"end with underscore", "project_", true},
		{"special characters", "project@test", true},
		{"spaces", "my project", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectName(tt.value)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePathParameters(t *testing.T) {
	tests := []struct {
		name      string
		params    httprouter.Params
		shouldErr bool
	}{
		{
			"valid project ID",
			httprouter.Params{{Key: "project_id", Value: "123"}},
			false,
		},
		{
			"valid collection ID",
			httprouter.Params{{Key: "collection_id", Value: "456"}},
			false,
		},
		{
			"valid plan ID",
			httprouter.Params{{Key: "plan_id", Value: "789"}},
			false,
		},
		{
			"valid filename",
			httprouter.Params{{Key: "filename", Value: "test.jmx"}},
			false,
		},
		{
			"invalid project ID",
			httprouter.Params{{Key: "project_id", Value: "invalid"}},
			true,
		},
		{
			"invalid filename",
			httprouter.Params{{Key: "filename", Value: "../test.jmx"}},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathParameters(tt.params)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateQueryParameters(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		shouldErr bool
	}{
		{"valid limit", "limit=10", false},
		{"valid offset", "offset=20", false},
		{"valid sort", "sort=name", false},
		{"valid order", "order=asc", false},
		{"valid filter", "filter=status=active", false},
		{"invalid limit", "limit=invalid", true},
		{"invalid sort", "sort='; DROP TABLE", true},
		{"invalid filter", "filter='; DROP TABLE users; --", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, _ := url.Parse("http://example.com?" + tt.query)
			req := &http.Request{URL: u}
			
			err := validateQueryParameters(req)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateProjectJSON(t *testing.T) {
	tests := []struct {
		name      string
		data      interface{}
		shouldErr bool
	}{
		{
			"valid project",
			map[string]interface{}{"name": "myproject"},
			false,
		},
		{
			"invalid project name",
			map[string]interface{}{"name": "-invalid"},
			true,
		},
		{
			"non-string name",
			map[string]interface{}{"name": 123},
			true,
		},
		{
			"not an object",
			[]interface{}{1, 2, 3},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectJSON(tt.data)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCollectionJSON(t *testing.T) {
	tests := []struct {
		name      string
		data      interface{}
		shouldErr bool
	}{
		{
			"valid collection",
			map[string]interface{}{"tests": []interface{}{1, 2, 3}},
			false,
		},
		{
			"too many tests",
			map[string]interface{}{"tests": make([]interface{}, 101)},
			true,
		},
		{
			"non-array tests",
			map[string]interface{}{"tests": "not an array"},
			true,
		},
		{
			"not an object",
			"not an object",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCollectionJSON(tt.data)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePlanJSON(t *testing.T) {
	tests := []struct {
		name      string
		data      interface{}
		shouldErr bool
	}{
		{
			"valid plan",
			map[string]interface{}{"engines": float64(5)},
			false,
		},
		{
			"zero engines",
			map[string]interface{}{"engines": float64(0)},
			true,
		},
		{
			"too many engines",
			map[string]interface{}{"engines": float64(1001)},
			true,
		},
		{
			"non-numeric engines",
			map[string]interface{}{"engines": "not a number"},
			true,
		},
		{
			"not an object",
			42,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlanJSON(tt.data)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequestSize(t *testing.T) {
	tests := []struct {
		name      string
		size      int64
		shouldErr bool
	}{
		{"small request", 1024, false},
		{"medium request", MaxRequestBodySize / 2, false},
		{"max size request", MaxRequestBodySize, false},
		{"oversized request", MaxRequestBodySize + 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				ContentLength: tt.size,
				Body:          http.NoBody,
			}
			
			err := validateRequestSize(req)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Benchmark tests
func BenchmarkValidateID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		validateID("123456")
	}
}

func BenchmarkValidateFilename(b *testing.B) {
	for i := 0; i < b.N; i++ {
		validateFilename("test-file.jmx")
	}
}

func BenchmarkValidateGenericParam(b *testing.B) {
	for i := 0; i < b.N; i++ {
		validateGenericParam("some parameter value")
	}
}