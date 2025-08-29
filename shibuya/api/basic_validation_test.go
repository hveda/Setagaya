package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateIDBasic(t *testing.T) {
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

func TestValidateFilenameBasic(t *testing.T) {
	tests := []struct {
		name      string
		filename  string
		shouldErr bool
	}{
		{"valid filename", "test.txt", false},
		{"valid with numbers", "test123.xml", false},
		{"empty filename", "", true},
		{"path traversal", "../test.txt", true},
		{"forward slash", "path/test.txt", true},
		{"backslash", "path\\test.txt", true},
		{"null byte", "test\x00.txt", true},
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

func TestValidateProjectNameBasic(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		shouldErr bool
	}{
		{"valid name", "myproject", false},
		{"with numbers", "project123", false},
		{"with dash", "my-project", false},
		{"with underscore", "my_project", false},
		{"empty", "", true},
		{"start with dash", "-project", true},
		{"end with dash", "project-", true},
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