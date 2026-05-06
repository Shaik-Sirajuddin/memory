package provider

import (
	"errors"
	"fmt"
)

var NoProcessFound = errors.New("no process found")

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

func errorsNew(text string) error {
	return fmt.Errorf("%s", text)
}
