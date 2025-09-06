package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDBError(t *testing.T) {
	testCases := []struct {
		name         string
		originalErr  error
		message      string
		expectedStr  string
	}{
		{
			name:        "simple error with message",
			originalErr: errors.New("database connection failed"),
			message:     "Unable to connect to database",
			expectedStr: "Unable to connect to database",
		},
		{
			name:        "empty message",
			originalErr: errors.New("some error"),
			message:     "",
			expectedStr: "",
		},
		{
			name:        "nil original error",
			originalErr: nil,
			message:     "Custom error message",
			expectedStr: "Custom error message",
		},
		{
			name:        "complex error message",
			originalErr: errors.New("SQL syntax error"),
			message:     "Failed to execute query: invalid table name",
			expectedStr: "Failed to execute query: invalid table name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dbErr := &DBError{
				Err:     tc.originalErr,
				Message: tc.message,
			}

			// Test Error() method
			assert.Equal(t, tc.expectedStr, dbErr.Error())

			// Test that it implements error interface
			var err error = dbErr
			assert.Equal(t, tc.expectedStr, err.Error())
		})
	}
}

func TestDBErrorAsError(t *testing.T) {
	// Test that DBError can be used with errors.As
	originalErr := errors.New("original error")
	dbErr := &DBError{
		Err:     originalErr,
		Message: "database error occurred",
	}

	var targetDBErr *DBError
	assert.True(t, errors.As(dbErr, &targetDBErr))
	assert.Equal(t, "database error occurred", targetDBErr.Message)
	assert.Equal(t, originalErr, targetDBErr.Err)
}

func TestDBErrorUnwrap(t *testing.T) {
	// Test that we can identify the nature of DBError by its content
	originalErr := errors.New("connection timeout")
	dbErr := &DBError{
		Err:     originalErr,
		Message: "database timeout",
	}

	// While DBError doesn't implement Unwrap, we can still access the original error
	assert.Equal(t, originalErr, dbErr.Err)
	assert.Contains(t, dbErr.Err.Error(), "connection timeout")
}

func TestDBErrorComparison(t *testing.T) {
	// Test that DBError instances can be compared properly
	err1 := &DBError{
		Err:     errors.New("test error"),
		Message: "test message",
	}

	err2 := &DBError{
		Err:     errors.New("test error"),
		Message: "test message",
	}

	// Different instances with same content should not be equal
	assert.NotEqual(t, err1, err2)

	// Same instance should be equal to itself
	assert.Equal(t, err1, err1)
}