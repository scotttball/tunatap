package utils

import (
	"math"
	"math/rand"
	"time"
)

// BackoffConfig configures exponential backoff with jitter.
type BackoffConfig struct {
	// InitialInterval is the starting backoff duration.
	InitialInterval time.Duration

	// MaxInterval is the maximum backoff duration.
	MaxInterval time.Duration

	// Multiplier is the factor by which the interval increases each attempt.
	Multiplier float64

	// JitterFactor adds randomness to prevent thundering herd.
	// 0.3 means +/- 30% variation.
	JitterFactor float64

	// MaxAttempts is the maximum number of retry attempts (0 = unlimited).
	MaxAttempts int
}

// DefaultBackoffConfig returns sensible defaults for network operations.
func DefaultBackoffConfig() *BackoffConfig {
	return &BackoffConfig{
		InitialInterval: 2 * time.Second,
		MaxInterval:     2 * time.Minute,
		Multiplier:      2.0,
		JitterFactor:    0.3,
		MaxAttempts:     10,
	}
}

// AggressiveBackoffConfig returns config for operations that should retry quickly.
func AggressiveBackoffConfig() *BackoffConfig {
	return &BackoffConfig{
		InitialInterval: 500 * time.Millisecond,
		MaxInterval:     30 * time.Second,
		Multiplier:      1.5,
		JitterFactor:    0.2,
		MaxAttempts:     15,
	}
}

// ConservativeBackoffConfig returns config for expensive operations.
func ConservativeBackoffConfig() *BackoffConfig {
	return &BackoffConfig{
		InitialInterval: 5 * time.Second,
		MaxInterval:     5 * time.Minute,
		Multiplier:      2.0,
		JitterFactor:    0.3,
		MaxAttempts:     5,
	}
}

// CalculateBackoff returns the backoff duration for a given attempt number.
// Attempt numbers start at 0.
func (c *BackoffConfig) CalculateBackoff(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Calculate base exponential backoff
	base := float64(c.InitialInterval) * math.Pow(c.Multiplier, float64(attempt))

	// Cap at maximum interval
	if base > float64(c.MaxInterval) {
		base = float64(c.MaxInterval)
	}

	// Add jitter: duration * (1 +/- jitterFactor)
	// Using math/rand is fine here - jitter doesn't need cryptographic randomness
	jitterRange := c.JitterFactor * base
	jitter := (rand.Float64()*2 - 1) * jitterRange //nolint:gosec // jitter doesn't need crypto rand

	result := time.Duration(base + jitter)

	// Ensure we never return negative duration
	if result < 0 {
		result = time.Duration(base)
	}

	return result
}

// ShouldRetry returns true if another retry attempt should be made.
func (c *BackoffConfig) ShouldRetry(attempt int) bool {
	if c.MaxAttempts <= 0 {
		return true // unlimited retries
	}
	return attempt < c.MaxAttempts
}

// Backoff is a helper that manages retry state.
type Backoff struct {
	config  *BackoffConfig
	attempt int
}

// NewBackoff creates a new Backoff helper with the given config.
func NewBackoff(config *BackoffConfig) *Backoff {
	if config == nil {
		config = DefaultBackoffConfig()
	}
	return &Backoff{
		config:  config,
		attempt: 0,
	}
}

// Next returns the next backoff duration and increments the attempt counter.
// Returns the duration and whether more retries are allowed.
func (b *Backoff) Next() (time.Duration, bool) {
	if !b.config.ShouldRetry(b.attempt) {
		return 0, false
	}
	duration := b.config.CalculateBackoff(b.attempt)
	b.attempt++
	return duration, true
}

// Reset resets the attempt counter to zero.
func (b *Backoff) Reset() {
	b.attempt = 0
}

// Attempt returns the current attempt number.
func (b *Backoff) Attempt() int {
	return b.attempt
}

// Sleep sleeps for the next backoff duration.
// Returns false if no more retries are allowed.
func (b *Backoff) Sleep() bool {
	duration, ok := b.Next()
	if !ok {
		return false
	}
	time.Sleep(duration)
	return true
}
