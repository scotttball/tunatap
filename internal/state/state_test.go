package state

import (
	"sync"
	"testing"
)

func TestGetInstance(t *testing.T) {
	// GetInstance should always return the same instance
	instance1 := GetInstance()
	instance2 := GetInstance()

	if instance1 != instance2 {
		t.Error("GetInstance() should return the same instance")
	}

	if instance1 == nil {
		t.Error("GetInstance() should not return nil")
	}
}

func TestHomePath(t *testing.T) {
	state := GetInstance()

	// Set and get home path
	testPath := "/test/home/path"
	state.SetHomePath(testPath)

	got := state.GetHomePath()
	if got != testPath {
		t.Errorf("GetHomePath() = %q, want %q", got, testPath)
	}
}

func TestTenancy(t *testing.T) {
	state := GetInstance()

	// Clear existing tenancies for clean test
	state.mu.Lock()
	state.tenancies = make(map[string]*string)
	state.mu.Unlock()

	// Test SetTenancy and GetTenancyByName
	tenancyName := "test-tenancy"
	tenancyOcid := "ocid1.tenancy.oc1..testocid"

	state.SetTenancy(tenancyName, &tenancyOcid)

	got, ok := state.GetTenancyByName(tenancyName)
	if !ok {
		t.Errorf("GetTenancyByName(%q) returned false, want true", tenancyName)
	}
	if got == nil || *got != tenancyOcid {
		t.Errorf("GetTenancyByName(%q) = %v, want %q", tenancyName, got, tenancyOcid)
	}

	// Test non-existent tenancy
	_, ok = state.GetTenancyByName("non-existent")
	if ok {
		t.Error("GetTenancyByName for non-existent tenancy should return false")
	}
}

func TestSetTenancies(t *testing.T) {
	state := GetInstance()

	// Clear existing tenancies
	state.mu.Lock()
	state.tenancies = make(map[string]*string)
	state.mu.Unlock()

	// Set multiple tenancies
	ocid1 := "ocid1.tenancy.oc1..test1"
	ocid2 := "ocid1.tenancy.oc1..test2"

	tenancies := map[string]*string{
		"tenancy1": &ocid1,
		"tenancy2": &ocid2,
	}

	state.SetTenancies(tenancies)

	// Verify both were set
	got1, ok1 := state.GetTenancyByName("tenancy1")
	got2, ok2 := state.GetTenancyByName("tenancy2")

	if !ok1 || got1 == nil || *got1 != ocid1 {
		t.Errorf("tenancy1 not set correctly: got %v", got1)
	}
	if !ok2 || got2 == nil || *got2 != ocid2 {
		t.Errorf("tenancy2 not set correctly: got %v", got2)
	}
}

func TestGetAllTenancies(t *testing.T) {
	state := GetInstance()

	// Clear existing tenancies
	state.mu.Lock()
	state.tenancies = make(map[string]*string)
	state.mu.Unlock()

	// Set tenancies
	ocid1 := "ocid1.tenancy.oc1..test1"
	ocid2 := "ocid1.tenancy.oc1..test2"

	state.SetTenancy("t1", &ocid1)
	state.SetTenancy("t2", &ocid2)

	all := state.GetAllTenancies()

	if len(all) != 2 {
		t.Errorf("GetAllTenancies() returned %d tenancies, want 2", len(all))
	}

	// Verify it's a copy (modifying shouldn't affect state)
	all["t3"] = &ocid1
	_, ok := state.GetTenancyByName("t3")
	if ok {
		t.Error("GetAllTenancies() should return a copy, not the original map")
	}
}

func TestConcurrentAccess(t *testing.T) {
	state := GetInstance()

	// Clear existing tenancies
	state.mu.Lock()
	state.tenancies = make(map[string]*string)
	state.mu.Unlock()

	var wg sync.WaitGroup
	const numGoroutines = 100

	// Test concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ocid := "ocid1.tenancy.oc1..test"
			state.SetTenancy("concurrent-test", &ocid)
			state.GetTenancyByName("concurrent-test")
			state.GetHomePath()
			state.SetHomePath("/test/path")
		}(i)
	}

	wg.Wait()

	// If we get here without race conditions, the test passes
	t.Log("Concurrent access test passed")
}

func TestNilTenancyValue(t *testing.T) {
	state := GetInstance()

	// Clear existing tenancies
	state.mu.Lock()
	state.tenancies = make(map[string]*string)
	state.mu.Unlock()

	// Setting nil value should work
	state.SetTenancy("nil-test", nil)

	got, ok := state.GetTenancyByName("nil-test")
	if !ok {
		t.Error("GetTenancyByName should return true for nil value")
	}
	if got != nil {
		t.Error("GetTenancyByName should return nil for nil value")
	}
}
