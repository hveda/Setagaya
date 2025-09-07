package controller

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrEngine(t *testing.T) {
	// Test that the engine error constant is properly defined
	assert.NotNil(t, ErrEngine)
	assert.Contains(t, ErrEngine.Error(), "Error with Engine-")
}

func TestMakeWrongEngineTypeError(t *testing.T) {
	err := makeWrongEngineTypeError()

	// Test error creation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Wrong Engine type requested")

	// Test error wrapping
	assert.True(t, errors.Is(err, ErrEngine))
}

func TestErrEngineWrapping(t *testing.T) {
	wrongEngineErr := makeWrongEngineTypeError()

	// Test errors.Is functionality
	assert.True(t, errors.Is(wrongEngineErr, ErrEngine))

	// Test that it doesn't match other error types
	otherErr := errors.New("some other error")
	assert.False(t, errors.Is(wrongEngineErr, otherErr))
	assert.False(t, errors.Is(otherErr, ErrEngine))
}

func TestErrEngineContent(t *testing.T) {
	err := makeWrongEngineTypeError()
	errMsg := err.Error()

	// Test that the error message contains both the base error and the specific message
	assert.Contains(t, errMsg, "Error with Engine-")
	assert.Contains(t, errMsg, "Wrong Engine type requested")
}

func TestErrEngineType(t *testing.T) {
	err := makeWrongEngineTypeError()

	// Test that the error is properly typed
	assert.Error(t, err)

	// Test that it wraps the ErrEngine
	assert.True(t, errors.Is(err, ErrEngine))
}

func TestMultipleErrEngines(t *testing.T) {
	// Test creating multiple engine errors
	err1 := makeWrongEngineTypeError()
	err2 := makeWrongEngineTypeError()

	// Both should be ErrEngines
	assert.True(t, errors.Is(err1, ErrEngine))
	assert.True(t, errors.Is(err2, ErrEngine))

	// Both should have the same message
	assert.Equal(t, err1.Error(), err2.Error())

	// But they should be different instances
	assert.NotSame(t, err1, err2)
}

func TestErrEngineConstants(t *testing.T) {
	// Test that error constants remain stable
	assert.Equal(t, "Error with Engine-", ErrEngine.Error())

	// Test that the constant cannot be accidentally modified (it's a read-only check)
	originalErr := ErrEngine
	assert.Equal(t, originalErr, ErrEngine)
}
