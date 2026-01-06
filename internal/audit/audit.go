package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// EventType represents the type of audit event.
type EventType string

const (
	EventTypeConnect    EventType = "connect"
	EventTypeDisconnect EventType = "disconnect"
	EventTypeError      EventType = "error"
	EventTypeRefresh    EventType = "session_refresh"
	EventTypeExec       EventType = "exec"
)

// AuditEvent represents a single audit log entry.
type AuditEvent struct {
	Timestamp   time.Time         `json:"timestamp"`
	EventType   EventType         `json:"event_type"`
	SessionID   string            `json:"session_id,omitempty"`
	ClusterName string            `json:"cluster_name,omitempty"`
	Region      string            `json:"region,omitempty"`
	LocalPort   int               `json:"local_port,omitempty"`
	RemoteHost  string            `json:"remote_host,omitempty"`
	RemotePort  int               `json:"remote_port,omitempty"`
	BastionID   string            `json:"bastion_id,omitempty"`
	Duration    *time.Duration    `json:"duration_ns,omitempty"`
	Error       string            `json:"error,omitempty"`
	Command     string            `json:"command,omitempty"`
	ExitCode    *int              `json:"exit_code,omitempty"`
	User        string            `json:"user,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Session tracks an active tunnel session for audit purposes.
type Session struct {
	ID          string
	ClusterName string
	Region      string
	LocalPort   int
	RemoteHost  string
	RemotePort  int
	BastionID   string
	StartTime   time.Time
	metadata    map[string]string
}

// Logger handles audit logging to a local file.
type Logger struct {
	logPath   string
	mu        sync.Mutex
	file      *os.File
	sessions  map[string]*Session
	sessionMu sync.RWMutex
}

// NewLogger creates a new audit logger.
func NewLogger(logDir string) (*Logger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create log file path with date
	logPath := filepath.Join(logDir, fmt.Sprintf("audit-%s.jsonl", time.Now().Format("2006-01-02")))

	// Open log file in append mode
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &Logger{
		logPath:  logPath,
		file:     file,
		sessions: make(map[string]*Session),
	}, nil
}

// Close closes the audit logger.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Log writes an audit event to the log file.
func (l *Logger) Log(event *AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Get current user
	if event.User == "" {
		event.User = os.Getenv("USER")
	}

	// Marshal to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Write to file with newline
	if _, err := l.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	// Sync to ensure durability
	return l.file.Sync()
}

// StartSession starts tracking a new session.
func (l *Logger) StartSession(session *Session) error {
	if session.ID == "" {
		session.ID = generateSessionID()
	}
	session.StartTime = time.Now().UTC()

	l.sessionMu.Lock()
	l.sessions[session.ID] = session
	l.sessionMu.Unlock()

	// Log connect event
	return l.Log(&AuditEvent{
		EventType:   EventTypeConnect,
		SessionID:   session.ID,
		ClusterName: session.ClusterName,
		Region:      session.Region,
		LocalPort:   session.LocalPort,
		RemoteHost:  session.RemoteHost,
		RemotePort:  session.RemotePort,
		BastionID:   session.BastionID,
		Metadata:    session.metadata,
	})
}

// EndSession ends a tracked session.
func (l *Logger) EndSession(sessionID string, errorMsg string) error {
	l.sessionMu.Lock()
	session, exists := l.sessions[sessionID]
	if exists {
		delete(l.sessions, sessionID)
	}
	l.sessionMu.Unlock()

	if !exists {
		log.Warn().Str("session_id", sessionID).Msg("Session not found for audit")
		return nil
	}

	duration := time.Since(session.StartTime)

	eventType := EventTypeDisconnect
	if errorMsg != "" {
		eventType = EventTypeError
	}

	return l.Log(&AuditEvent{
		EventType:   eventType,
		SessionID:   sessionID,
		ClusterName: session.ClusterName,
		Region:      session.Region,
		LocalPort:   session.LocalPort,
		RemoteHost:  session.RemoteHost,
		RemotePort:  session.RemotePort,
		BastionID:   session.BastionID,
		Duration:    &duration,
		Error:       errorMsg,
		Metadata:    session.metadata,
	})
}

// LogSessionRefresh logs a session refresh event.
func (l *Logger) LogSessionRefresh(sessionID, newBastionSessionID string) error {
	l.sessionMu.RLock()
	session, exists := l.sessions[sessionID]
	l.sessionMu.RUnlock()

	if !exists {
		return nil
	}

	return l.Log(&AuditEvent{
		EventType:   EventTypeRefresh,
		SessionID:   sessionID,
		ClusterName: session.ClusterName,
		Region:      session.Region,
		BastionID:   session.BastionID,
		Metadata: map[string]string{
			"new_bastion_session": newBastionSessionID,
		},
	})
}

// LogExec logs a command execution event.
func (l *Logger) LogExec(sessionID, clusterName, command string, exitCode int, duration time.Duration) error {
	return l.Log(&AuditEvent{
		EventType:   EventTypeExec,
		SessionID:   sessionID,
		ClusterName: clusterName,
		Command:     command,
		ExitCode:    &exitCode,
		Duration:    &duration,
	})
}

// GetActiveSessions returns all active sessions.
func (l *Logger) GetActiveSessions() []*Session {
	l.sessionMu.RLock()
	defer l.sessionMu.RUnlock()

	sessions := make([]*Session, 0, len(l.sessions))
	for _, s := range l.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// generateSessionID generates a unique session ID.
func generateSessionID() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())
}

// DefaultLogDir returns the audit log directory, respecting configured home path.
func DefaultLogDir() string {
	// Check if a custom home path is configured via state
	if homePath := getConfiguredHomePath(); homePath != "" {
		return filepath.Join(homePath, "audit")
	}
	// Fall back to default ~/.tunatap/audit
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/tunatap/audit"
	}
	return filepath.Join(home, ".tunatap", "audit")
}

// getConfiguredHomePath returns the configured tunatap home path if set.
// This avoids import cycles by using a package-level variable that can be set.
var configuredHomePath string

// SetHomePath allows setting the audit log home path from external packages.
func SetHomePath(path string) {
	configuredHomePath = path
}

func getConfiguredHomePath() string {
	return configuredHomePath
}

// Query represents criteria for querying audit logs.
type Query struct {
	StartTime   *time.Time
	EndTime     *time.Time
	ClusterName string
	EventType   EventType
	SessionID   string
	Limit       int
}

// QueryLogs queries audit logs based on criteria.
func QueryLogs(logDir string, q Query) ([]AuditEvent, error) {
	// Find all log files
	pattern := filepath.Join(logDir, "audit-*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to find log files: %w", err)
	}

	events := make([]AuditEvent, 0)

	for _, file := range files {
		fileEvents, err := readLogFile(file, q)
		if err != nil {
			log.Warn().Err(err).Str("file", file).Msg("Failed to read log file")
			continue
		}
		events = append(events, fileEvents...)
	}

	// Apply limit
	if q.Limit > 0 && len(events) > q.Limit {
		events = events[len(events)-q.Limit:]
	}

	return events, nil
}

// readLogFile reads and filters events from a single log file.
func readLogFile(path string, q Query) ([]AuditEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	events := make([]AuditEvent, 0)
	decoder := json.NewDecoder(file)

	for decoder.More() {
		var event AuditEvent
		if err := decoder.Decode(&event); err != nil {
			continue // Skip malformed entries
		}

		// Apply filters
		if q.StartTime != nil && event.Timestamp.Before(*q.StartTime) {
			continue
		}
		if q.EndTime != nil && event.Timestamp.After(*q.EndTime) {
			continue
		}
		if q.ClusterName != "" && event.ClusterName != q.ClusterName {
			continue
		}
		if q.EventType != "" && event.EventType != q.EventType {
			continue
		}
		if q.SessionID != "" && event.SessionID != q.SessionID {
			continue
		}

		events = append(events, event)
	}

	return events, nil
}

// FormatEvent formats an audit event for display.
func FormatEvent(e *AuditEvent) string {
	ts := e.Timestamp.Local().Format("2006-01-02 15:04:05")

	switch e.EventType {
	case EventTypeConnect:
		return fmt.Sprintf("[%s] CONNECT  %s -> localhost:%d (session: %s)",
			ts, e.ClusterName, e.LocalPort, e.SessionID)
	case EventTypeDisconnect:
		duration := "unknown"
		if e.Duration != nil {
			duration = e.Duration.Round(time.Second).String()
		}
		return fmt.Sprintf("[%s] DISCONNECT %s (duration: %s, session: %s)",
			ts, e.ClusterName, duration, e.SessionID)
	case EventTypeError:
		return fmt.Sprintf("[%s] ERROR    %s: %s (session: %s)",
			ts, e.ClusterName, e.Error, e.SessionID)
	case EventTypeRefresh:
		return fmt.Sprintf("[%s] REFRESH  %s (session: %s)",
			ts, e.ClusterName, e.SessionID)
	case EventTypeExec:
		exitCode := ""
		if e.ExitCode != nil {
			exitCode = fmt.Sprintf(" (exit: %d)", *e.ExitCode)
		}
		return fmt.Sprintf("[%s] EXEC     %s: %s%s",
			ts, e.ClusterName, e.Command, exitCode)
	default:
		return fmt.Sprintf("[%s] %s %s", ts, e.EventType, e.ClusterName)
	}
}

// Summary represents aggregated audit statistics.
type Summary struct {
	TotalConnections int
	TotalDuration    time.Duration
	ClusterStats     map[string]ClusterStat
	ErrorCount       int
}

// ClusterStat contains stats for a single cluster.
type ClusterStat struct {
	ConnectionCount int
	TotalDuration   time.Duration
	ErrorCount      int
	LastAccess      time.Time
}

// GetSummary generates a summary from audit events.
func GetSummary(events []AuditEvent) *Summary {
	summary := &Summary{
		ClusterStats: make(map[string]ClusterStat),
	}

	for i := range events {
		switch events[i].EventType {
		case EventTypeConnect:
			summary.TotalConnections++
			stat := summary.ClusterStats[events[i].ClusterName]
			stat.ConnectionCount++
			if events[i].Timestamp.After(stat.LastAccess) {
				stat.LastAccess = events[i].Timestamp
			}
			summary.ClusterStats[events[i].ClusterName] = stat

		case EventTypeDisconnect:
			if events[i].Duration != nil {
				summary.TotalDuration += *events[i].Duration
				stat := summary.ClusterStats[events[i].ClusterName]
				stat.TotalDuration += *events[i].Duration
				summary.ClusterStats[events[i].ClusterName] = stat
			}

		case EventTypeError:
			summary.ErrorCount++
			stat := summary.ClusterStats[events[i].ClusterName]
			stat.ErrorCount++
			summary.ClusterStats[events[i].ClusterName] = stat
		}
	}

	return summary
}
