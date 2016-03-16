package util

import "testing"

func TestRetry(t *testing.T) {
	opts := &RetryOption{
		MaxAttempts: 20,
	}
	attempts := 0
	for r := NewRetry(opts); r.Next(); attempts++ {
		if attempts > opts.MaxAttempts {
			t.Fatalf("MaxAttempts exceeds.")
		}
	}

	r := NewRetry(opts)
	for r.Next() { /* NULL */ }
	r.Reset()
	if !r.Next() {
		t.Fatalf("Retry.Next() should return true after reset.")
	}

	stopc := make(chan struct{}, 1)

	opts.MaxAttempts = 0
	opts.Stopper = stopc

	attempts = 0
	for r := NewRetry(opts); r.Next(); attempts++ {
		if attempts == 20 {
			stopc <- struct{}{}
		}
		if attempts > 20 {
			t.Fatalf("Retry can't stop with Stopper correctly.")
			break
		}
	}
}
