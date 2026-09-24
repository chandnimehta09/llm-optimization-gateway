package telemetry

import (
	"context"
	"fmt"
)

// ShutdownFunc is a function that performs graceful shutdown of a telemetry component.
type ShutdownFunc func(ctx context.Context) error

// noopShutdown is a shutdown function that does nothing.
var noopShutdown ShutdownFunc = func(_ context.Context) error {
	return nil
}

// CombineShutdownFuncs combines multiple shutdown functions into a single one.
// All functions are called in reverse order (LIFO). Errors are collected and
// returned as a combined error.
func CombineShutdownFuncs(fns ...ShutdownFunc) ShutdownFunc {
	return func(ctx context.Context) error {
		var errs []error
		// Shut down in reverse order (last initialized = first shutdown).
		for i := len(fns) - 1; i >= 0; i-- {
			if fns[i] != nil {
				if err := fns[i](ctx); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if len(errs) == 0 {
			return nil
		}
		return combineErrors(errs)
	}
}

// combineErrors joins multiple errors into a single error.
func combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	msg := "multiple shutdown errors:"
	for _, err := range errs {
		msg += fmt.Sprintf(" [%v]", err)
	}
	return fmt.Errorf("%s", msg)
}
