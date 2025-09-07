package scheduler

import (
	"errors"
	"fmt"
)

var (
	ErrIngress = errors.New("Error with Ingress-")
)

func makeSchedulerIngressError(err error) error {
	return fmt.Errorf("%w%s", ErrIngress, err.Error())
}

func makeIPNotAssignedError() error {
	return fmt.Errorf("%w%s", ErrIngress, "IP is not assigned yet")
}

type NoResourcesFoundErr struct {
	Err     error
	Message string
}

func (e *NoResourcesFoundErr) Error() string {
	return e.Message
}
