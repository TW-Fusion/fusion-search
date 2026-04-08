package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/sony/gobreaker"
)

// HTTPStatusError represents an HTTP error with status code.
type HTTPStatusError struct {
	StatusCode int
	Message    string
}

func (e *HTTPStatusError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("http status error: %d", e.StatusCode)
}

func newCircuitBreaker(name string, cfg *config.AppConfig) *gobreaker.CircuitBreaker {
	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: 1,
		Interval:    0,
		Timeout:     time.Duration(cfg.Resilience.CircuitBreakerRecoveryTimeout) * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return int(counts.ConsecutiveFailures) >= cfg.Resilience.CircuitBreakerFailureThreshold
		},
	}
	return gobreaker.NewCircuitBreaker(settings)
}

func retryWithBackoff[T any](
	ctx context.Context,
	maxAttempts int,
	backoffBase float64,
	retryStatusCodes []int,
	fn func() (T, error),
) (T, error) {
	var zero T
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if backoffBase <= 0 {
		backoffBase = 0.5
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		value, err := fn()
		if err == nil {
			return value, nil
		}
		lastErr = err

		if attempt == maxAttempts || !isRetryableError(err, retryStatusCodes) {
			break
		}

		multiplier := 1 << (attempt - 1)
		delay := time.Duration(float64(time.Second) * backoffBase * float64(multiplier))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return zero, ctx.Err()
		}
	}
	return zero, lastErr
}

func isRetryableError(err error, retryStatusCodes []int) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
	}

	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		for _, code := range retryStatusCodes {
			if statusErr.StatusCode == code {
				return true
			}
		}
	}

	return false
}
