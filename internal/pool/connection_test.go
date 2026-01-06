package pool

import (
	"sync"
	"testing"
)

func TestNewTrackedConnection(t *testing.T) {
	// We can't easily create a real SSH client in tests, so test with nil
	conn := NewTrackedConnection(nil, 10)

	if conn == nil {
		t.Fatal("NewTrackedConnection returned nil")
	}

	if conn.maxUses != 10 {
		t.Errorf("maxUses = %d, want 10", conn.maxUses)
	}

	if conn.useCount != 0 {
		t.Errorf("initial useCount = %d, want 0", conn.useCount)
	}

	if conn.invalid {
		t.Error("initial invalid = true, want false")
	}
}

func TestTrackedConnectionIncrement(t *testing.T) {
	conn := NewTrackedConnection(nil, 3)

	// Should be able to increment up to maxUses
	for i := 0; i < 3; i++ {
		if !conn.Increment() {
			t.Errorf("Increment() failed at count %d", i+1)
		}
	}

	// Should not be able to increment beyond maxUses
	if conn.Increment() {
		t.Error("Increment() should return false when at max uses")
	}

	if conn.GetUseCount() != 3 {
		t.Errorf("GetUseCount() = %d, want 3", conn.GetUseCount())
	}
}

func TestTrackedConnectionDecrement(t *testing.T) {
	conn := NewTrackedConnection(nil, 5)

	// Increment a few times
	conn.Increment()
	conn.Increment()
	conn.Increment()

	if conn.GetUseCount() != 3 {
		t.Fatalf("GetUseCount() after increments = %d, want 3", conn.GetUseCount())
	}

	// Decrement
	conn.Decrement()
	if conn.GetUseCount() != 2 {
		t.Errorf("GetUseCount() after decrement = %d, want 2", conn.GetUseCount())
	}

	// Decrement below zero should not go negative
	conn.Decrement()
	conn.Decrement()
	conn.Decrement() // Extra decrement

	if conn.GetUseCount() != 0 {
		t.Errorf("GetUseCount() should not go below 0, got %d", conn.GetUseCount())
	}
}

func TestTrackedConnectionInvalidate(t *testing.T) {
	conn := NewTrackedConnection(nil, 5)

	if conn.IsInvalid() {
		t.Error("New connection should not be invalid")
	}

	conn.Invalidate()

	if !conn.IsInvalid() {
		t.Error("Connection should be invalid after Invalidate()")
	}

	// Should not be able to increment invalid connection
	if conn.Increment() {
		t.Error("Increment() should return false for invalid connection")
	}
}

func TestTrackedConnectionCanAcceptMore(t *testing.T) {
	conn := NewTrackedConnection(nil, 2)

	if !conn.CanAcceptMore() {
		t.Error("New connection should be able to accept more")
	}

	conn.Increment()
	if !conn.CanAcceptMore() {
		t.Error("Connection with 1/2 uses should accept more")
	}

	conn.Increment()
	if conn.CanAcceptMore() {
		t.Error("Connection at max uses should not accept more")
	}

	// Invalidated connection should not accept more
	conn2 := NewTrackedConnection(nil, 5)
	conn2.Invalidate()
	if conn2.CanAcceptMore() {
		t.Error("Invalid connection should not accept more")
	}
}

func TestTrackedConnectionIsIdle(t *testing.T) {
	conn := NewTrackedConnection(nil, 5)

	if !conn.IsIdle() {
		t.Error("New connection should be idle")
	}

	conn.Increment()
	if conn.IsIdle() {
		t.Error("Connection with use count > 0 should not be idle")
	}

	conn.Decrement()
	if !conn.IsIdle() {
		t.Error("Connection with use count 0 should be idle")
	}
}

func TestTrackedConnectionClose(t *testing.T) {
	conn := NewTrackedConnection(nil, 5)

	err := conn.Close()
	if err != nil {
		t.Errorf("Close() with nil client error = %v", err)
	}

	if !conn.IsInvalid() {
		t.Error("Connection should be invalid after Close()")
	}
}

func TestTrackedConnectionConcurrency(t *testing.T) {
	conn := NewTrackedConnection(nil, 100)

	var wg sync.WaitGroup
	const numGoroutines = 50

	// Concurrent increments
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn.Increment()
		}()
	}
	wg.Wait()

	if conn.GetUseCount() != numGoroutines {
		t.Errorf("After concurrent increments, count = %d, want %d", conn.GetUseCount(), numGoroutines)
	}

	// Concurrent decrements
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn.Decrement()
		}()
	}
	wg.Wait()

	if conn.GetUseCount() != 0 {
		t.Errorf("After concurrent decrements, count = %d, want 0", conn.GetUseCount())
	}
}

func TestTrackedConnectionMixedConcurrency(t *testing.T) {
	conn := NewTrackedConnection(nil, 1000)

	var wg sync.WaitGroup
	const iterations = 100

	// Mixed concurrent operations
	for i := 0; i < iterations; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			conn.Increment()
		}()
		go func() {
			defer wg.Done()
			conn.GetUseCount()
		}()
		go func() {
			defer wg.Done()
			conn.IsInvalid()
		}()
		go func() {
			defer wg.Done()
			conn.CanAcceptMore()
		}()
	}

	wg.Wait()

	// If we get here without race conditions, test passes
	t.Log("Mixed concurrency test passed")
}
