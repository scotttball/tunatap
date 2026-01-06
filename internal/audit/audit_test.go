package audit

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("NewLogger error: %v", err)
	}
	defer logger.Close()

	// Verify log file was created
	pattern := filepath.Join(tempDir, "audit-*.jsonl")
	files, _ := filepath.Glob(pattern)
	if len(files) != 1 {
		t.Errorf("Expected 1 log file, got %d", len(files))
	}
}

func TestLog(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("NewLogger error: %v", err)
	}
	defer logger.Close()

	// Log an event
	event := &AuditEvent{
		EventType:   EventTypeConnect,
		SessionID:   "test-session-1",
		ClusterName: "test-cluster",
		Region:      "us-ashburn-1",
		LocalPort:   6443,
	}

	if err := logger.Log(event); err != nil {
		t.Fatalf("Log error: %v", err)
	}

	// Verify event was logged
	events, err := QueryLogs(tempDir, Query{})
	if err != nil {
		t.Fatalf("QueryLogs error: %v", err)
	}

	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}

	if events[0].ClusterName != "test-cluster" {
		t.Errorf("ClusterName = %q, want %q", events[0].ClusterName, "test-cluster")
	}
}

func TestStartAndEndSession(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("NewLogger error: %v", err)
	}
	defer logger.Close()

	session := &Session{
		ClusterName: "test-cluster",
		Region:      "us-ashburn-1",
		LocalPort:   6443,
		RemoteHost:  "10.0.0.100",
		RemotePort:  6443,
	}

	// Start session
	if err := logger.StartSession(session); err != nil {
		t.Fatalf("StartSession error: %v", err)
	}

	if session.ID == "" {
		t.Error("Session ID should be set")
	}

	// Verify session is tracked
	activeSessions := logger.GetActiveSessions()
	if len(activeSessions) != 1 {
		t.Errorf("Expected 1 active session, got %d", len(activeSessions))
	}

	// End session
	time.Sleep(10 * time.Millisecond) // Small delay for duration
	if err := logger.EndSession(session.ID, ""); err != nil {
		t.Fatalf("EndSession error: %v", err)
	}

	// Verify session was removed
	activeSessions = logger.GetActiveSessions()
	if len(activeSessions) != 0 {
		t.Errorf("Expected 0 active sessions, got %d", len(activeSessions))
	}

	// Verify events were logged
	events, err := QueryLogs(tempDir, Query{})
	if err != nil {
		t.Fatalf("QueryLogs error: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

func TestEndSessionWithError(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("NewLogger error: %v", err)
	}
	defer logger.Close()

	session := &Session{
		ClusterName: "test-cluster",
	}

	logger.StartSession(session)
	logger.EndSession(session.ID, "connection lost")

	events, _ := QueryLogs(tempDir, Query{EventType: EventTypeError})
	if len(events) != 1 {
		t.Errorf("Expected 1 error event, got %d", len(events))
	}
}

func TestQueryLogsWithFilters(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("NewLogger error: %v", err)
	}
	defer logger.Close()

	// Log multiple events
	logger.Log(&AuditEvent{
		EventType:   EventTypeConnect,
		ClusterName: "cluster-a",
	})
	logger.Log(&AuditEvent{
		EventType:   EventTypeConnect,
		ClusterName: "cluster-b",
	})
	logger.Log(&AuditEvent{
		EventType:   EventTypeError,
		ClusterName: "cluster-a",
	})

	// Filter by cluster
	events, _ := QueryLogs(tempDir, Query{ClusterName: "cluster-a"})
	if len(events) != 2 {
		t.Errorf("Expected 2 events for cluster-a, got %d", len(events))
	}

	// Filter by event type
	events, _ = QueryLogs(tempDir, Query{EventType: EventTypeConnect})
	if len(events) != 2 {
		t.Errorf("Expected 2 connect events, got %d", len(events))
	}

	// Filter with limit
	events, _ = QueryLogs(tempDir, Query{Limit: 1})
	if len(events) != 1 {
		t.Errorf("Expected 1 event with limit, got %d", len(events))
	}
}

func TestQueryLogsTimeFilter(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("NewLogger error: %v", err)
	}
	defer logger.Close()

	// Log event
	now := time.Now()
	logger.Log(&AuditEvent{
		Timestamp:   now,
		EventType:   EventTypeConnect,
		ClusterName: "test",
	})

	// Query with start time after event
	future := now.Add(time.Hour)
	events, _ := QueryLogs(tempDir, Query{StartTime: &future})
	if len(events) != 0 {
		t.Errorf("Expected 0 events after start time, got %d", len(events))
	}

	// Query with start time before event
	past := now.Add(-time.Hour)
	events, _ = QueryLogs(tempDir, Query{StartTime: &past})
	if len(events) != 1 {
		t.Errorf("Expected 1 event before start time, got %d", len(events))
	}
}

func TestFormatEvent(t *testing.T) {
	tests := []struct {
		event    *AuditEvent
		contains string
	}{
		{
			event:    &AuditEvent{EventType: EventTypeConnect, ClusterName: "test", LocalPort: 6443},
			contains: "CONNECT",
		},
		{
			event:    &AuditEvent{EventType: EventTypeDisconnect, ClusterName: "test"},
			contains: "DISCONNECT",
		},
		{
			event:    &AuditEvent{EventType: EventTypeError, ClusterName: "test", Error: "connection lost"},
			contains: "ERROR",
		},
		{
			event:    &AuditEvent{EventType: EventTypeRefresh, ClusterName: "test"},
			contains: "REFRESH",
		},
		{
			event:    &AuditEvent{EventType: EventTypeExec, ClusterName: "test", Command: "kubectl"},
			contains: "EXEC",
		},
	}

	for _, tt := range tests {
		formatted := FormatEvent(tt.event)
		if formatted == "" {
			t.Errorf("FormatEvent returned empty for %v", tt.event.EventType)
		}
	}
}

func TestGetSummary(t *testing.T) {
	duration := 10 * time.Minute
	events := []AuditEvent{
		{EventType: EventTypeConnect, ClusterName: "cluster-a"},
		{EventType: EventTypeConnect, ClusterName: "cluster-a"},
		{EventType: EventTypeConnect, ClusterName: "cluster-b"},
		{EventType: EventTypeDisconnect, ClusterName: "cluster-a", Duration: &duration},
		{EventType: EventTypeError, ClusterName: "cluster-a", Error: "timeout"},
	}

	summary := GetSummary(events)

	if summary.TotalConnections != 3 {
		t.Errorf("TotalConnections = %d, want 3", summary.TotalConnections)
	}

	if summary.TotalDuration != duration {
		t.Errorf("TotalDuration = %v, want %v", summary.TotalDuration, duration)
	}

	if summary.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", summary.ErrorCount)
	}

	if summary.ClusterStats["cluster-a"].ConnectionCount != 2 {
		t.Errorf("cluster-a connections = %d, want 2", summary.ClusterStats["cluster-a"].ConnectionCount)
	}

	if summary.ClusterStats["cluster-b"].ConnectionCount != 1 {
		t.Errorf("cluster-b connections = %d, want 1", summary.ClusterStats["cluster-b"].ConnectionCount)
	}
}

func TestDefaultLogDir(t *testing.T) {
	dir := DefaultLogDir()
	if dir == "" {
		t.Error("DefaultLogDir returned empty string")
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	if id1 == "" {
		t.Error("generateSessionID returned empty string")
	}

	// IDs should contain process ID
	if id1 == id2 {
		t.Error("generateSessionID should return unique IDs")
	}
}

func TestLogExec(t *testing.T) {
	tempDir := t.TempDir()

	logger, err := NewLogger(tempDir)
	if err != nil {
		t.Fatalf("NewLogger error: %v", err)
	}
	defer logger.Close()

	err = logger.LogExec("session-1", "test-cluster", "kubectl get pods", 0, 5*time.Second)
	if err != nil {
		t.Fatalf("LogExec error: %v", err)
	}

	events, _ := QueryLogs(tempDir, Query{EventType: EventTypeExec})
	if len(events) != 1 {
		t.Errorf("Expected 1 exec event, got %d", len(events))
	}

	if events[0].Command != "kubectl get pods" {
		t.Errorf("Command = %q, want %q", events[0].Command, "kubectl get pods")
	}
}
