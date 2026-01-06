package utils

import (
	"testing"
)

func TestStringPtr(t *testing.T) {
	s := "hello"
	ptr := StringPtr(s)

	if ptr == nil {
		t.Fatal("StringPtr returned nil")
	}
	if *ptr != s {
		t.Errorf("StringPtr returned %q, want %q", *ptr, s)
	}
}

func TestBoolPtr(t *testing.T) {
	tests := []bool{true, false}

	for _, b := range tests {
		ptr := BoolPtr(b)
		if ptr == nil {
			t.Fatal("BoolPtr returned nil")
		}
		if *ptr != b {
			t.Errorf("BoolPtr returned %v, want %v", *ptr, b)
		}
	}
}

func TestIntPtr(t *testing.T) {
	tests := []int{0, 1, -1, 100, -100}

	for _, i := range tests {
		ptr := IntPtr(i)
		if ptr == nil {
			t.Fatal("IntPtr returned nil")
		}
		if *ptr != i {
			t.Errorf("IntPtr returned %d, want %d", *ptr, i)
		}
	}
}
