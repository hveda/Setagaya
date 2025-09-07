package scheduler

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIngressError(t *testing.T) {
	// Test the ErrIngress constant
	assert.Error(t, ErrIngress)
	assert.Equal(t, "Error with Ingress-", ErrIngress.Error())
}

func TestMakeSchedulerIngressError(t *testing.T) {
	testCases := []struct {
		name          string
		inputError    error
		expectedMsg   string
		shouldWrapErr bool
	}{
		{
			name:          "simple error",
			inputError:    errors.New("connection failed"),
			expectedMsg:   "Error with Ingress-connection failed",
			shouldWrapErr: true,
		},
		{
			name:          "empty error message",
			inputError:    errors.New(""),
			expectedMsg:   "Error with Ingress-",
			shouldWrapErr: true,
		},
		{
			name:          "complex error message",
			inputError:    errors.New("timeout after 30 seconds waiting for pods"),
			expectedMsg:   "Error with Ingress-timeout after 30 seconds waiting for pods",
			shouldWrapErr: true,
		},
		{
			name:          "error with special characters",
			inputError:    errors.New("failed: status=500, message=\"internal error\""),
			expectedMsg:   "Error with Ingress-failed: status=500, message=\"internal error\"",
			shouldWrapErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := makeSchedulerIngressError(tc.inputError)

			assert.Error(t, result)
			assert.Equal(t, tc.expectedMsg, result.Error())

			if tc.shouldWrapErr {
				// Test that the original error is wrapped
				assert.True(t, errors.Is(result, ErrIngress))
			}
		})
	}
}

func TestMakeIPNotAssignedError(t *testing.T) {
	result := makeIPNotAssignedError()

	assert.Error(t, result)
	assert.Equal(t, "Error with Ingress-IP is not assigned yet", result.Error())

	// Test that it wraps ErrIngress
	assert.True(t, errors.Is(result, ErrIngress))
}

func TestNoResourcesFoundErr(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		message  string
		expected string
	}{
		{
			name:     "simple error",
			err:      errors.New("not found"),
			message:  "No resources found",
			expected: "No resources found",
		},
		{
			name:     "empty message",
			err:      errors.New("some error"),
			message:  "",
			expected: "",
		},
		{
			name:     "nil error with message",
			err:      nil,
			message:  "Custom message",
			expected: "Custom message",
		},
		{
			name:     "complex message",
			err:      errors.New("database timeout"),
			message:  "No pods found matching criteria: collection=123, plan=456",
			expected: "No pods found matching criteria: collection=123, plan=456",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			noResourcesErr := &NoResourcesFoundErr{
				Err:     tc.err,
				Message: tc.message,
			}

			// Test Error() method
			assert.Equal(t, tc.expected, noResourcesErr.Error())

			// Test that it implements error interface
			var err error = noResourcesErr
			assert.Equal(t, tc.expected, err.Error())

			// Test struct fields
			assert.Equal(t, tc.err, noResourcesErr.Err)
			assert.Equal(t, tc.message, noResourcesErr.Message)
		})
	}
}

func TestNoResourcesFoundErrAsError(t *testing.T) {
	// Test that NoResourcesFoundErr can be used with errors.As
	originalErr := errors.New("original error")
	noResourcesErr := &NoResourcesFoundErr{
		Err:     originalErr,
		Message: "resources not found",
	}

	var targetErr *NoResourcesFoundErr
	assert.True(t, errors.As(noResourcesErr, &targetErr))
	assert.Equal(t, "resources not found", targetErr.Message)
	assert.Equal(t, originalErr, targetErr.Err)
}

func TestErrorWrappingBehavior(t *testing.T) {
	// Test error wrapping behavior
	originalErr := errors.New("original ingress error")
	wrappedErr := makeSchedulerIngressError(originalErr)

	// Test that we can unwrap to get ErrIngress
	assert.True(t, errors.Is(wrappedErr, ErrIngress))

	// Test error chain
	assert.Contains(t, wrappedErr.Error(), ErrIngress.Error())
	assert.Contains(t, wrappedErr.Error(), originalErr.Error())
}

func TestErrorTypeComparison(t *testing.T) {
	// Test different error types can be distinguished
	ingressErr := makeSchedulerIngressError(errors.New("test"))
	ipErr := makeIPNotAssignedError()
	noResourcesErr := &NoResourcesFoundErr{
		Err:     errors.New("test"),
		Message: "test message",
	}

	// All should be different error types
	assert.NotEqual(t, ingressErr, ipErr)
	assert.NotEqual(t, ingressErr, noResourcesErr)
	assert.NotEqual(t, ipErr, noResourcesErr)

	// But ingress errors should wrap the same base error
	assert.True(t, errors.Is(ingressErr, ErrIngress))
	assert.True(t, errors.Is(ipErr, ErrIngress))
	assert.False(t, errors.Is(noResourcesErr, ErrIngress))
}

func TestNoResourcesFoundErrEdgeCases(t *testing.T) {
	t.Run("nil struct", func(t *testing.T) {
		var noResourcesErr *NoResourcesFoundErr
		assert.Panics(t, func() {
			_ = noResourcesErr.Error()
		})
	})

	t.Run("zero value struct", func(t *testing.T) {
		noResourcesErr := NoResourcesFoundErr{}
		assert.Equal(t, "", noResourcesErr.Error())
		assert.Nil(t, noResourcesErr.Err)
	})

	t.Run("struct with only error", func(t *testing.T) {
		noResourcesErr := NoResourcesFoundErr{
			Err: errors.New("some error"),
		}
		assert.Equal(t, "", noResourcesErr.Error()) // Message takes precedence
	})

	t.Run("struct with only message", func(t *testing.T) {
		noResourcesErr := NoResourcesFoundErr{
			Message: "only message",
		}
		assert.Equal(t, "only message", noResourcesErr.Error())
		assert.Nil(t, noResourcesErr.Err)
	})
}

func TestIngressErrorConstants(t *testing.T) {
	// Test that ErrIngress is a sentinel error that can be compared
	err1 := ErrIngress
	err2 := ErrIngress

	assert.Equal(t, err1, err2)
	assert.True(t, errors.Is(err1, err2))
	assert.True(t, errors.Is(err2, err1))
}

func TestErrorMessages(t *testing.T) {
	// Test specific error message formats
	testErr := errors.New("test failure")

	t.Run("scheduler ingress error format", func(t *testing.T) {
		err := makeSchedulerIngressError(testErr)
		expected := "Error with Ingress-test failure"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("IP not assigned error format", func(t *testing.T) {
		err := makeIPNotAssignedError()
		expected := "Error with Ingress-IP is not assigned yet"
		assert.Equal(t, expected, err.Error())
	})
}

func TestErrorInterfaces(t *testing.T) {
	// Test that all error types implement the error interface
	var err error

	err = ErrIngress
	assert.NotNil(t, err)

	err = makeSchedulerIngressError(errors.New("test"))
	assert.NotNil(t, err)

	err = makeIPNotAssignedError()
	assert.NotNil(t, err)

	err = &NoResourcesFoundErr{Message: "test"}
	assert.NotNil(t, err)
}
