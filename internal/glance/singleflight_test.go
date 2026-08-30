package glance

import (
	"errors"
	"testing"
)

func TestSingleflightReturnsValue(t *testing.T) {
	call := NewSingleflight(func() (string, error) {
		return "result", nil
	})

	value, err := call()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if value != "result" {
		t.Fatalf("expected result, got %q", value)
	}
}

func TestSingleflightReturnsError(t *testing.T) {
	expectedErr := errors.New("test error")

	call := NewSingleflight(func() (string, error) {
		return "", expectedErr
	})

	_, err := call()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got %v", expectedErr, err)
	}
}

func TestSingleflightWaitsForCurrentCall(t *testing.T) {
	expectedErr := errors.New("shared error")

	current := &SingleflightCall[string]{
		done: make(chan struct{}),
	}

	fnCalled := false

	sf := &Singleflight[string]{
		fn: func() (string, error) {
			fnCalled = true
			return "unexpected", nil
		},
		current: current,
	}

	type callResult struct {
		value string
		err   error
	}

	resultCh := make(chan callResult, 1)

	go func() {
		value, err := sf.Do()
		resultCh <- callResult{
			value: value,
			err:   err,
		}
	}()

	current.val = "shared result"
	current.err = expectedErr
	close(current.done)

	got := <-resultCh

	if fnCalled {
		t.Fatal("expected current call to be shared instead of invoking function")
	}

	if got.value != "shared result" {
		t.Fatalf("expected shared result, got %q", got.value)
	}

	if !errors.Is(got.err, expectedErr) {
		t.Fatalf("expected shared error %v, got %v", expectedErr, got.err)
	}
}

func TestSingleflightStartsNewInvocationAfterCompletion(t *testing.T) {
	invocations := 0

	call := NewSingleflight(func() (int, error) {
		invocations++
		return invocations, nil
	})

	first, err := call()
	if err != nil {
		t.Fatalf("first call returned unexpected error: %v", err)
	}

	second, err := call()
	if err != nil {
		t.Fatalf("second call returned unexpected error: %v", err)
	}

	if first != 1 {
		t.Fatalf("expected first invocation result 1, got %d", first)
	}

	if second != 2 {
		t.Fatalf("expected second invocation result 2, got %d", second)
	}

	if invocations != 2 {
		t.Fatalf("expected two separate invocations, got %d", invocations)
	}
}
