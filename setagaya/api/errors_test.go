package api

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMakeLoginError(t *testing.T) {
	err := makeLoginError()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "you need to login")
	assert.True(t, errors.Is(err, errNoPermission))
}

func TestMakeInvalidRequestError(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "simple message",
			message:  "invalid input",
			expected: "invalid input",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
		{
			name:     "message with special characters",
			message:  "field 'name' is required and must be non-empty",
			expected: "field 'name' is required and must be non-empty",
		},
		{
			name:     "long message",
			message:  "this is a very long error message that should still work correctly",
			expected: "this is a very long error message that should still work correctly",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := makeInvalidRequestError(tc.message)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expected)
			assert.True(t, errors.Is(err, errInvalidRequest))
		})
	}
}

func TestMakeNoPermissionErr(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "access denied message",
			message:  "access denied to resource",
			expected: "access denied to resource",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
		{
			name:     "permission message",
			message:  "insufficient permissions",
			expected: "insufficient permissions",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := makeNoPermissionErr(tc.message)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expected)
			assert.True(t, errors.Is(err, errNoPermission))
		})
	}
}

func TestMakeInternalErrServeror(t *testing.T) {
	testCases := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "database error",
			message:  "database connection failed",
			expected: "database connection failed",
		},
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
		{
			name:     "service error",
			message:  "external service unavailable",
			expected: "external service unavailable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := makeInternalServerError(tc.message)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expected)
			assert.True(t, errors.Is(err, ErrServer))
		})
	}
}

func TestMakeInvalidResourceError(t *testing.T) {
	testCases := []struct {
		name     string
		resource string
		expected string
	}{
		{
			name:     "project resource",
			resource: "project",
			expected: "invalid project",
		},
		{
			name:     "collection resource",
			resource: "collection",
			expected: "invalid collection",
		},
		{
			name:     "plan resource",
			resource: "plan",
			expected: "invalid plan",
		},
		{
			name:     "empty resource",
			resource: "",
			expected: "invalid ",
		},
		{
			name:     "resource with spaces",
			resource: "test resource",
			expected: "invalid test resource",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := makeInvalidResourceError(tc.resource)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expected)
			assert.True(t, errors.Is(err, errInvalidRequest))
		})
	}
}

func TestMakeProjectOwnershipError(t *testing.T) {
	err := makeProjectOwnershipError()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "You don't own the project")
	assert.True(t, errors.Is(err, errNoPermission))
}

func TestMakeCollectionOwnershipError(t *testing.T) {
	err := makeCollectionOwnershipError()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "You don't own the collection")
	assert.True(t, errors.Is(err, errNoPermission))
}

func TestErrorConstants(t *testing.T) {
	// Test that error constants are properly defined
	assert.NotNil(t, errNoPermission)
	assert.NotNil(t, errInvalidRequest)
	assert.NotNil(t, ErrServer)

	// Test error constant values
	assert.Contains(t, errNoPermission.Error(), "403-")
	assert.Contains(t, errInvalidRequest.Error(), "400-")
	assert.Contains(t, ErrServer.Error(), "500-")
}

func TestErrorWrapping(t *testing.T) {
	// Test that errors properly wrap base errors for error type checking
	loginErr := makeLoginError()
	invalidErr := makeInvalidRequestError("test")
	permissionErr := makeNoPermissionErr("test")
	serverErr := makeInternalServerError("test")
	resourceErr := makeInvalidResourceError("test")
	projectOwnershipErr := makeProjectOwnershipError()
	collectionOwnershipErr := makeCollectionOwnershipError()

	// Test error.Is() functionality
	assert.True(t, errors.Is(loginErr, errNoPermission))
	assert.True(t, errors.Is(invalidErr, errInvalidRequest))
	assert.True(t, errors.Is(permissionErr, errNoPermission))
	assert.True(t, errors.Is(serverErr, ErrServer))
	assert.True(t, errors.Is(resourceErr, errInvalidRequest))
	assert.True(t, errors.Is(projectOwnershipErr, errNoPermission))
	assert.True(t, errors.Is(collectionOwnershipErr, errNoPermission))

	// Test cross-type error checking (should be false)
	assert.False(t, errors.Is(loginErr, errInvalidRequest))
	assert.False(t, errors.Is(invalidErr, errNoPermission))
	assert.False(t, errors.Is(serverErr, errInvalidRequest))
}
