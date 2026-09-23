package telemetry

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCombineShutdownFuncs_NoFuncs(t *testing.T) {
	combined := CombineShutdownFuncs()
	if err := combined(context.Background()); err != nil {
		t.Errorf("CombineShutdownFuncs() with no funcs returned error: %v", err)
	}
}

func TestCombineShutdownFuncs_SingleSuccess(t *testing.T) {
	called := false
	fn := func(_ context.Context) error {
		called = true
		return nil
	}

	combined := CombineShutdownFuncs(fn)
	if err := combined(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("shutdown function was not called")
	}
}

func TestCombineShutdownFuncs_MultipleSuccess(t *testing.T) {
	var order []int
	fn1 := func(_ context.Context) error {
		order = append(order, 1)
		return nil
	}
	fn2 := func(_ context.Context) error {
		order = append(order, 2)
		return nil
	}
	fn3 := func(_ context.Context) error {
		order = append(order, 3)
		return nil
	}

	combined := CombineShutdownFuncs(fn1, fn2, fn3)
	if err := combined(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should be called in reverse order (LIFO).
	if len(order) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(order))
	}
	if order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Errorf("expected LIFO order [3,2,1], got %v", order)
	}
}

func TestCombineShutdownFuncs_WithErrors(t *testing.T) {
	fn1 := func(_ context.Context) error {
		return errors.New("error1")
	}
	fn2 := func(_ context.Context) error {
		return nil
	}
	fn3 := func(_ context.Context) error {
		return errors.New("error3")
	}

	combined := CombineShutdownFuncs(fn1, fn2, fn3)
	err := combined(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "error3") || !strings.Contains(errMsg, "error1") {
		t.Errorf("error message should contain both errors, got: %s", errMsg)
	}
}

func TestCombineShutdownFuncs_NilFuncsSkipped(t *testing.T) {
	called := false
	fn := func(_ context.Context) error {
		called = true
		return nil
	}

	combined := CombineShutdownFuncs(nil, fn, nil)
	if err := combined(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !called {
		t.Error("non-nil shutdown function was not called")
	}
}

func TestNoopShutdown(t *testing.T) {
	if err := noopShutdown(context.Background()); err != nil {
		t.Errorf("noopShutdown() returned error: %v", err)
	}
}
