package domain

import "time"

// RateLimitError means the provider rejected the operation and it is safe to retry after the delay.
type RateLimitError struct {
	Err        error
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string { return e.Err.Error() }
func (e *RateLimitError) Unwrap() error { return e.Err }
