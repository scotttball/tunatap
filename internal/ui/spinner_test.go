package ui

import (
	"errors"
	"testing"
	"time"
)

func TestSpinner_StartStop(t *testing.T) {
	s := NewSpinner("Testing")
	s.Start()
	time.Sleep(200 * time.Millisecond)
	s.Stop()
}

func TestSpinner_StopWithMessage(t *testing.T) {
	s := NewSpinner("Testing")
	s.Start()
	time.Sleep(100 * time.Millisecond)
	s.StopWithMessage("Done")
}

func TestSpinner_UpdateMessage(t *testing.T) {
	s := NewSpinner("Initial")
	s.Start()
	time.Sleep(100 * time.Millisecond)
	s.UpdateMessage("Updated")
	time.Sleep(100 * time.Millisecond)
	s.Stop()
}

func TestSpinner_DoubleStart(t *testing.T) {
	s := NewSpinner("Testing")
	s.Start()
	s.Start() // Should be safe to call twice
	time.Sleep(100 * time.Millisecond)
	s.Stop()
}

func TestSpinner_DoubleStop(t *testing.T) {
	s := NewSpinner("Testing")
	s.Start()
	time.Sleep(100 * time.Millisecond)
	s.Stop()
	s.Stop() // Should be safe to call twice
}

func TestRunWithSpinner_Success(t *testing.T) {
	err := RunWithSpinner("Testing operation", func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Errorf("RunWithSpinner returned error: %v", err)
	}
}

func TestRunWithSpinner_Error(t *testing.T) {
	expectedErr := errors.New("test error")
	err := RunWithSpinner("Testing operation", func() error {
		time.Sleep(100 * time.Millisecond)
		return expectedErr
	})
	if err != expectedErr {
		t.Errorf("RunWithSpinner returned %v, want %v", err, expectedErr)
	}
}

func TestRunWithSpinnerResult_Success(t *testing.T) {
	result, err := RunWithSpinnerResult("Testing operation", func() (string, error) {
		time.Sleep(100 * time.Millisecond)
		return "success", nil
	})
	if err != nil {
		t.Errorf("RunWithSpinnerResult returned error: %v", err)
	}
	if result != "success" {
		t.Errorf("RunWithSpinnerResult returned %q, want %q", result, "success")
	}
}

func TestProgress_Update(t *testing.T) {
	p := NewProgress("Loading", 100)
	for i := 0; i <= 100; i += 10 {
		p.Update(i)
	}
	p.Done()
}

func TestIsTerminal(t *testing.T) {
	// This test just verifies the function doesn't panic
	_ = IsTerminal()
}
