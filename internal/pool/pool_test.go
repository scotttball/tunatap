package pool

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
)

// mockFactory creates a factory function for testing
func mockFactory(shouldFail bool, callCount *int32) ConnectionFactory {
	return func() (*ssh.Client, error) {
		if callCount != nil {
			atomic.AddInt32(callCount, 1)
		}
		if shouldFail {
			return nil, errors.New("mock connection failure")
		}
		// Return nil client - tests don't need real SSH connections
		return nil, nil
	}
}

func TestNewConnectionPool(t *testing.T) {
	var callCount int32
	factory := mockFactory(false, &callCount)

	pool, err := NewConnectionPool(5, 10, factory, 2)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	if pool == nil {
		t.Fatal("NewConnectionPool() returned nil")
	}

	// Should have created warmup connections
	if pool.Size() != 2 {
		t.Errorf("Pool size after warmup = %d, want 2", pool.Size())
	}

	if callCount != 2 {
		t.Errorf("Factory called %d times, want 2", callCount)
	}
}

func TestNewConnectionPoolNoWarmup(t *testing.T) {
	var callCount int32
	factory := mockFactory(false, &callCount)

	pool, err := NewConnectionPool(5, 10, factory, 0)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	if pool.Size() != 0 {
		t.Errorf("Pool size with no warmup = %d, want 0", pool.Size())
	}

	if callCount != 0 {
		t.Errorf("Factory called %d times with no warmup, want 0", callCount)
	}
}

func TestNewConnectionPoolWarmupFailure(t *testing.T) {
	factory := mockFactory(true, nil)

	_, err := NewConnectionPool(5, 10, factory, 2)
	if err == nil {
		t.Error("NewConnectionPool() should error when all warmup connections fail")
	}
}

func TestConnectionPoolGet(t *testing.T) {
	factory := mockFactory(false, nil)

	pool, err := NewConnectionPool(5, 10, factory, 1)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	// Get a connection
	conn, err := pool.Get()
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if conn == nil {
		t.Fatal("Get() returned nil connection")
	}

	// Use count should be 1
	if conn.GetUseCount() != 1 {
		t.Errorf("Connection use count = %d, want 1", conn.GetUseCount())
	}
}

func TestConnectionPoolGetReuse(t *testing.T) {
	var callCount int32
	factory := mockFactory(false, &callCount)

	pool, err := NewConnectionPool(5, 10, factory, 1)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	// Get multiple connections - should reuse the existing one
	conn1, _ := pool.Get()
	conn2, _ := pool.Get()
	conn3, _ := pool.Get()

	// All should be the same connection
	if conn1 != conn2 || conn2 != conn3 {
		t.Error("Pool should reuse existing connections")
	}

	// Factory should only have been called once for warmup
	if callCount != 1 {
		t.Errorf("Factory called %d times, want 1", callCount)
	}

	// Use count should be 3
	if conn1.GetUseCount() != 3 {
		t.Errorf("Connection use count = %d, want 3", conn1.GetUseCount())
	}
}

func TestConnectionPoolGetCreatesNew(t *testing.T) {
	var callCount int32
	factory := mockFactory(false, &callCount)

	// Pool with max concurrent use of 2
	pool, err := NewConnectionPool(5, 2, factory, 1)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	// Get 2 connections - should use same one
	pool.Get()
	pool.Get()

	// Pool should still have 1 connection
	if pool.Size() != 1 {
		t.Errorf("Pool size = %d, want 1", pool.Size())
	}

	// Get another - should create new connection
	pool.Get()

	if pool.Size() != 2 {
		t.Errorf("Pool size after exceeding max concurrent = %d, want 2", pool.Size())
	}
}

func TestConnectionPoolSize(t *testing.T) {
	factory := mockFactory(false, nil)

	pool, err := NewConnectionPool(5, 10, factory, 3)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	if pool.Size() != 3 {
		t.Errorf("Size() = %d, want 3", pool.Size())
	}
}

func TestConnectionPoolActiveCount(t *testing.T) {
	factory := mockFactory(false, nil)

	pool, err := NewConnectionPool(5, 10, factory, 1)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	if pool.ActiveCount() != 0 {
		t.Errorf("Initial ActiveCount() = %d, want 0", pool.ActiveCount())
	}

	conn1, _ := pool.Get()
	pool.Get()

	if pool.ActiveCount() != 2 {
		t.Errorf("ActiveCount() after 2 Gets = %d, want 2", pool.ActiveCount())
	}

	conn1.Decrement()

	if pool.ActiveCount() != 1 {
		t.Errorf("ActiveCount() after decrement = %d, want 1", pool.ActiveCount())
	}
}

func TestConnectionPoolClose(t *testing.T) {
	factory := mockFactory(false, nil)

	pool, err := NewConnectionPool(5, 10, factory, 2)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}

	pool.Close()

	if pool.Size() != 0 {
		t.Errorf("Size() after Close() = %d, want 0", pool.Size())
	}
}

func TestConnectionPoolHealthCheck(t *testing.T) {
	factory := mockFactory(false, nil)

	pool, err := NewConnectionPool(5, 10, factory, 2)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	// Health check with always-healthy function
	pool.HealthCheck(func(c *ssh.Client) bool {
		return true
	})

	if pool.Size() != 2 {
		t.Errorf("Size() after healthy check = %d, want 2", pool.Size())
	}

	// Health check that marks all as unhealthy
	pool.HealthCheck(func(c *ssh.Client) bool {
		return false
	})

	// Connections should be marked invalid but not removed if in use
	// Since they're not in use, they should be removed
	if pool.Size() != 0 {
		t.Errorf("Size() after unhealthy check = %d, want 0", pool.Size())
	}
}

func TestConnectionPoolConcurrentGet(t *testing.T) {
	var callCount int32
	factory := mockFactory(false, &callCount)

	pool, err := NewConnectionPool(10, 100, factory, 2)
	if err != nil {
		t.Fatalf("NewConnectionPool() error = %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	const numGoroutines = 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := pool.Get()
			if err != nil {
				t.Errorf("Concurrent Get() error = %v", err)
				return
			}
			if conn == nil {
				t.Error("Concurrent Get() returned nil")
			}
		}()
	}

	wg.Wait()

	// Should have gotten connections for all goroutines
	if pool.ActiveCount() != numGoroutines {
		t.Errorf("ActiveCount() = %d, want %d", pool.ActiveCount(), numGoroutines)
	}
}

func TestCheckSSHClientHealth(t *testing.T) {
	// With nil client, should return false (can't send keepalive)
	// This is expected behavior - nil clients are unhealthy
	result := CheckSSHClientHealth(nil)
	if result {
		t.Error("CheckSSHClientHealth(nil) = true, want false")
	}
}
