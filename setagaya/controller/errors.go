package controller

import (
	"errors"
	"fmt"
)

var (
	ErrEngine = errors.New("error with Engine-")
)

func makeWrongEngineTypeError() error {
	return fmt.Errorf("%w%s", ErrEngine, "wrong engine type requested")
}
