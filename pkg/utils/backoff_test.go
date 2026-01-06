package utils

import (
	"testing"
	"time"
)

func TestDefaultBackoffConfig(t *testing.T) {
	cfg := DefaultBackoffConfig()

	if cfg.InitialInterval != 2*time.Second {
		t.Errorf("InitialInterval = %v, want %v", cfg.InitialInterval, 2*time.Second)
	}
	if cfg.MaxInterval != 2*time.Minute {
		t.Errorf("MaxInterval = %v, want %v", cfg.MaxInterval, 2*time.Minute)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want %v", cfg.Multiplier, 2.0)
	}
	if cfg.JitterFactor != 0.3 {
		t.Errorf("JitterFactor = %v, want %v", cfg.JitterFactor, 0.3)
	}
	if cfg.MaxAttempts != 10 {
		t.Errorf("MaxAttempts = %v, want %v", cfg.MaxAttempts, 10)
	}
}

func TestCalculateBackoff_ExponentialGrowth(t *testing.T) {
	cfg := &BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     1 * time.Hour, // High enough to not cap
		Multiplier:      2.0,
		JitterFactor:    0, // No jitter for predictable testing
		MaxAttempts:     10,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
	}

	for _, tt := range tests {
		result := cfg.CalculateBackoff(tt.attempt)
		if result != tt.expected {
			t.Errorf("CalculateBackoff(%d) = %v, want %v", tt.attempt, result, tt.expected)
		}
	}
}

func TestCalculateBackoff_MaxIntervalCap(t *testing.T) {
	cfg := &BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     10 * time.Second,
		Multiplier:      2.0,
		JitterFactor:    0,
		MaxAttempts:     20,
	}

	// Attempt 5 would be 32 seconds without cap, should be capped to 10
	result := cfg.CalculateBackoff(5)
	if result != 10*time.Second {
		t.Errorf("CalculateBackoff(5) = %v, want %v (capped)", result, 10*time.Second)
	}

	// Very high attempt should also be capped
	result = cfg.CalculateBackoff(100)
	if result != 10*time.Second {
		t.Errorf("CalculateBackoff(100) = %v, want %v (capped)", result, 10*time.Second)
	}
}

func TestCalculateBackoff_JitterBounds(t *testing.T) {
	cfg := &BackoffConfig{
		InitialInterval: 10 * time.Second,
		MaxInterval:     1 * time.Hour,
		Multiplier:      1.0, // No growth, just test jitter
		JitterFactor:    0.3,
		MaxAttempts:     10,
	}

	base := 10 * time.Second
	minExpected := time.Duration(float64(base) * 0.7) // -30%
	maxExpected := time.Duration(float64(base) * 1.3) // +30%

	// Run multiple times to test jitter distribution
	for i := 0; i < 100; i++ {
		result := cfg.CalculateBackoff(0)
		if result < minExpected || result > maxExpected {
			t.Errorf("CalculateBackoff(0) = %v, want between %v and %v",
				result, minExpected, maxExpected)
		}
	}
}

func TestCalculateBackoff_NegativeAttempt(t *testing.T) {
	cfg := DefaultBackoffConfig()
	cfg.JitterFactor = 0 // Disable jitter for predictable test

	// Negative attempt should be treated as 0
	result := cfg.CalculateBackoff(-5)
	expected := cfg.InitialInterval
	if result != expected {
		t.Errorf("CalculateBackoff(-5) = %v, want %v", result, expected)
	}
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name        string
		maxAttempts int
		attempt     int
		want        bool
	}{
		{"first attempt with limit", 5, 0, true},
		{"mid attempt with limit", 5, 3, true},
		{"at limit", 5, 5, false},
		{"over limit", 5, 10, false},
		{"unlimited first", 0, 0, true},
		{"unlimited high", 0, 1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &BackoffConfig{MaxAttempts: tt.maxAttempts}
			got := cfg.ShouldRetry(tt.attempt)
			if got != tt.want {
				t.Errorf("ShouldRetry(%d) with MaxAttempts=%d = %v, want %v",
					tt.attempt, tt.maxAttempts, got, tt.want)
			}
		})
	}
}

func TestBackoff_Next(t *testing.T) {
	cfg := &BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     1 * time.Hour,
		Multiplier:      2.0,
		JitterFactor:    0,
		MaxAttempts:     3,
	}

	b := NewBackoff(cfg)

	// First call
	d, ok := b.Next()
	if !ok || d != 1*time.Second {
		t.Errorf("First Next() = (%v, %v), want (1s, true)", d, ok)
	}
	if b.Attempt() != 1 {
		t.Errorf("Attempt() = %d, want 1", b.Attempt())
	}

	// Second call
	d, ok = b.Next()
	if !ok || d != 2*time.Second {
		t.Errorf("Second Next() = (%v, %v), want (2s, true)", d, ok)
	}

	// Third call
	d, ok = b.Next()
	if !ok || d != 4*time.Second {
		t.Errorf("Third Next() = (%v, %v), want (4s, true)", d, ok)
	}

	// Fourth call should fail (max 3 attempts)
	d, ok = b.Next()
	if ok {
		t.Errorf("Fourth Next() = (%v, %v), want (_, false)", d, ok)
	}
}

func TestBackoff_Reset(t *testing.T) {
	cfg := &BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     1 * time.Hour,
		Multiplier:      2.0,
		JitterFactor:    0,
		MaxAttempts:     3,
	}

	b := NewBackoff(cfg)

	// Use up some attempts
	b.Next()
	b.Next()

	if b.Attempt() != 2 {
		t.Errorf("Before reset: Attempt() = %d, want 2", b.Attempt())
	}

	b.Reset()

	if b.Attempt() != 0 {
		t.Errorf("After reset: Attempt() = %d, want 0", b.Attempt())
	}

	// Should get initial interval again
	d, ok := b.Next()
	if !ok || d != 1*time.Second {
		t.Errorf("After reset Next() = (%v, %v), want (1s, true)", d, ok)
	}
}

func TestBackoff_NilConfig(t *testing.T) {
	b := NewBackoff(nil)

	// Should use default config
	d, ok := b.Next()
	if !ok {
		t.Error("NewBackoff(nil).Next() returned false, want true")
	}
	if d < 1*time.Second || d > 3*time.Second {
		t.Errorf("NewBackoff(nil).Next() = %v, want ~2s (with jitter)", d)
	}
}

func TestAggressiveBackoffConfig(t *testing.T) {
	cfg := AggressiveBackoffConfig()

	if cfg.InitialInterval != 500*time.Millisecond {
		t.Errorf("InitialInterval = %v, want 500ms", cfg.InitialInterval)
	}
	if cfg.Multiplier != 1.5 {
		t.Errorf("Multiplier = %v, want 1.5", cfg.Multiplier)
	}
}

func TestConservativeBackoffConfig(t *testing.T) {
	cfg := ConservativeBackoffConfig()

	if cfg.InitialInterval != 5*time.Second {
		t.Errorf("InitialInterval = %v, want 5s", cfg.InitialInterval)
	}
	if cfg.MaxAttempts != 5 {
		t.Errorf("MaxAttempts = %v, want 5", cfg.MaxAttempts)
	}
}
