package glance

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPoolDoProcessesAllItems(t *testing.T) {
	task := func(input int) (int, error) {
		return input * 2, nil
	}

	results, errs, err := workerPoolDo(
		newJob(task, []int{1, 2, 3, 4}).withWorkers(2),
	)
	if err != nil {
		t.Fatalf("workerPoolDo returned unexpected error: %v", err)
	}

	expected := []int{2, 4, 6, 8}

	for i := range expected {
		if errs[i] != nil {
			t.Fatalf("item %d returned unexpected error: %v", i, errs[i])
		}

		if results[i] != expected[i] {
			t.Fatalf(
				"item %d returned %d, expected %d",
				i,
				results[i],
				expected[i],
			)
		}
	}
}

func TestWorkerPoolDoUsesBackgroundContextByDefault(t *testing.T) {
	job := newJob(
		func(input int) (int, error) {
			return input, nil
		},
		[]int{1},
	)

	if job.ctx == nil {
		t.Fatal("newJob created a nil context")
	}

	if err := job.ctx.Err(); err != nil {
		t.Fatalf("default job context unexpectedly has an error: %v", err)
	}
}

func TestWorkerPoolWithContextIgnoresNilContext(t *testing.T) {
	job := newJob(
		func(input int) (int, error) {
			return input, nil
		},
		[]int{1},
	)

	originalContext := job.ctx
	job.withContext(nil)

	if job.ctx != originalContext {
		t.Fatal("withContext(nil) replaced the existing context")
	}
}

func TestWorkerPoolDoReturnsCancelledBeforeSingleItemExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls atomic.Int32

	task := func(input int) (int, error) {
		calls.Add(1)
		return input, nil
	}

	results, errs, err := workerPoolDo(
		newJob(task, []int{1}).withContext(ctx),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if calls.Load() != 0 {
		t.Fatalf("task executed %d times after cancellation", calls.Load())
	}

	if len(results) != 1 || len(errs) != 1 {
		t.Fatalf(
			"unexpected result sizes: results=%d errs=%d",
			len(results),
			len(errs),
		)
	}
}

func TestWorkerPoolDoReturnsCancelledBeforeMultipleItemExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls atomic.Int32

	task := func(input int) (int, error) {
		calls.Add(1)
		return input, nil
	}

	_, _, err := workerPoolDo(
		newJob(task, []int{1, 2, 3}).withWorkers(2).withContext(ctx),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if calls.Load() != 0 {
		t.Fatalf("task executed %d times after cancellation", calls.Load())
	}
}

func TestWorkerPoolDoStopsSchedulingAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	var calls atomic.Int32

	task := func(input int) (int, error) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
		}

		return input, nil
	}

	done := make(chan error, 1)

	go func() {
		_, _, err := workerPoolDo(
			newJob(
				task,
				[]int{1, 2, 3, 4, 5},
			).withWorkers(1).withContext(ctx),
		)

		done <- err
	}()

	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}

	cancel()
	close(releaseFirst)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("workerPoolDo did not return after cancellation")
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"expected only the in-flight task to execute, got %d executions",
			got,
		)
	}
}

func TestWorkerPoolDoEmptyJobReturnsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, errs, err := workerPoolDo(
		newJob(
			func(input int) (int, error) {
				return input, nil
			},
			nil,
		).withContext(ctx),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if len(results) != 0 || len(errs) != 0 {
		t.Fatalf(
			"expected empty results, got results=%d errs=%d",
			len(results),
			len(errs),
		)
	}
}
