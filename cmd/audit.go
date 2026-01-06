package cmd

import (
	"fmt"
	"time"

	"github.com/scotttball/tunatap/internal/audit"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "View and manage audit logs",
	Long: `View and manage local audit logs for tunnel connections.

Audit logs track all tunnel connections, disconnections, and errors
with timestamps, durations, and cluster information.`,
}

var auditListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent audit events",
	RunE:  runAuditList,
}

var auditSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show audit summary statistics",
	RunE:  runAuditSummary,
}

var auditShowCmd = &cobra.Command{
	Use:   "show <session-id>",
	Short: "Show details for a specific session",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuditShow,
}

var (
	auditLimit       int
	auditCluster     string
	auditSince       string
	auditEventType   string
	auditJSON        bool
)

func init() {
	rootCmd.AddCommand(auditCmd)

	auditCmd.AddCommand(auditListCmd)
	auditCmd.AddCommand(auditSummaryCmd)
	auditCmd.AddCommand(auditShowCmd)

	auditListCmd.Flags().IntVarP(&auditLimit, "limit", "n", 50, "number of events to show")
	auditListCmd.Flags().StringVarP(&auditCluster, "cluster", "c", "", "filter by cluster name")
	auditListCmd.Flags().StringVar(&auditSince, "since", "", "show events since (e.g., '24h', '7d', '2024-01-01')")
	auditListCmd.Flags().StringVarP(&auditEventType, "type", "t", "", "filter by event type (connect, disconnect, error)")
	auditListCmd.Flags().BoolVar(&auditJSON, "json", false, "output as JSON")

	auditSummaryCmd.Flags().StringVar(&auditSince, "since", "7d", "summary period (e.g., '24h', '7d', '30d')")
}

func runAuditList(cmd *cobra.Command, args []string) error {
	logDir := audit.DefaultLogDir()

	// Build query
	q := audit.Query{
		ClusterName: auditCluster,
		Limit:       auditLimit,
	}

	if auditEventType != "" {
		q.EventType = audit.EventType(auditEventType)
	}

	if auditSince != "" {
		startTime, err := parseSince(auditSince)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		q.StartTime = startTime
	}

	// Query logs
	events, err := audit.QueryLogs(logDir, q)
	if err != nil {
		return fmt.Errorf("failed to query audit logs: %w", err)
	}

	if len(events) == 0 {
		fmt.Println("No audit events found")
		return nil
	}

	if auditJSON {
		// JSON output
		for _, e := range events {
			fmt.Printf("%+v\n", e)
		}
	} else {
		// Human-readable output
		for _, e := range events {
			fmt.Println(audit.FormatEvent(&e))
		}
	}

	return nil
}

func runAuditSummary(cmd *cobra.Command, args []string) error {
	logDir := audit.DefaultLogDir()

	// Parse since parameter
	startTime, err := parseSince(auditSince)
	if err != nil {
		return fmt.Errorf("invalid --since value: %w", err)
	}

	// Query logs
	q := audit.Query{StartTime: startTime}
	events, err := audit.QueryLogs(logDir, q)
	if err != nil {
		return fmt.Errorf("failed to query audit logs: %w", err)
	}

	if len(events) == 0 {
		fmt.Println("No audit events found for the specified period")
		return nil
	}

	// Generate summary
	summary := audit.GetSummary(events)

	fmt.Printf("Audit Summary (since %s)\n", startTime.Local().Format("2006-01-02 15:04"))
	fmt.Println("========================")
	fmt.Printf("Total connections: %d\n", summary.TotalConnections)
	fmt.Printf("Total time connected: %s\n", summary.TotalDuration.Round(time.Second))
	fmt.Printf("Errors: %d\n", summary.ErrorCount)
	fmt.Println()

	if len(summary.ClusterStats) > 0 {
		fmt.Println("By Cluster:")
		for name, stat := range summary.ClusterStats {
			fmt.Printf("  %s:\n", name)
			fmt.Printf("    Connections: %d\n", stat.ConnectionCount)
			fmt.Printf("    Total time: %s\n", stat.TotalDuration.Round(time.Second))
			if stat.ErrorCount > 0 {
				fmt.Printf("    Errors: %d\n", stat.ErrorCount)
			}
			if !stat.LastAccess.IsZero() {
				fmt.Printf("    Last access: %s\n", stat.LastAccess.Local().Format("2006-01-02 15:04"))
			}
		}
	}

	return nil
}

func runAuditShow(cmd *cobra.Command, args []string) error {
	sessionID := args[0]
	logDir := audit.DefaultLogDir()

	// Query for specific session
	q := audit.Query{SessionID: sessionID}
	events, err := audit.QueryLogs(logDir, q)
	if err != nil {
		return fmt.Errorf("failed to query audit logs: %w", err)
	}

	if len(events) == 0 {
		return fmt.Errorf("session '%s' not found", sessionID)
	}

	fmt.Printf("Session: %s\n", sessionID)
	fmt.Println("=========")
	for _, e := range events {
		fmt.Println(audit.FormatEvent(&e))
	}

	return nil
}

// parseSince parses a duration or date string.
func parseSince(s string) (*time.Time, error) {
	// Try duration format first (24h, 7d, etc.)
	if d, err := parseDuration(s); err == nil {
		t := time.Now().Add(-d)
		return &t, nil
	}

	// Try date format
	for _, layout := range []string{
		"2006-01-02",
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("cannot parse '%s' as duration or date", s)
}

// parseDuration parses a duration string with day support.
func parseDuration(s string) (time.Duration, error) {
	// Handle 'd' suffix for days
	if len(s) > 0 && s[len(s)-1] == 'd' {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}

	return time.ParseDuration(s)
}
