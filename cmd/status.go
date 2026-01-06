package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/scotttball/tunatap/internal/audit"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show active tunnel status",
	Long: `Display information about currently active SSH tunnels.

This command parses audit logs to find tunnels that have connected
but not yet disconnected, showing their current status and uptime.`,
	RunE: runStatus,
}

var (
	statusJSON    bool
	statusVerbose bool
)

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output as JSON")
	statusCmd.Flags().BoolVarP(&statusVerbose, "verbose", "v", false, "show additional details")
}

// ActiveTunnel represents an active tunnel connection.
type ActiveTunnel struct {
	SessionID   string        `json:"session_id"`
	ClusterName string        `json:"cluster_name"`
	Region      string        `json:"region,omitempty"`
	LocalPort   int           `json:"local_port"`
	RemoteHost  string        `json:"remote_host"`
	RemotePort  int           `json:"remote_port"`
	BastionID   string        `json:"bastion_id,omitempty"`
	StartTime   time.Time     `json:"start_time"`
	Uptime      time.Duration `json:"uptime_ns"`
	UptimeStr   string        `json:"uptime"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	logDir := audit.DefaultLogDir()

	// Query recent events to find active tunnels
	// Look at events from the last 24 hours
	since := time.Now().Add(-24 * time.Hour)
	q := audit.Query{
		StartTime: &since,
		Limit:     1000, // Reasonable limit
	}

	events, err := audit.QueryLogs(logDir, q)
	if err != nil {
		return fmt.Errorf("failed to query audit logs: %w", err)
	}

	// Find active tunnels (connects without matching disconnects)
	activeTunnels := findActiveTunnels(events)

	if len(activeTunnels) == 0 {
		if statusJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("No active tunnels")
		}
		return nil
	}

	if statusJSON {
		return outputJSON(activeTunnels)
	}

	return outputTable(activeTunnels)
}

// findActiveTunnels finds tunnels that have connected but not disconnected.
func findActiveTunnels(events []audit.AuditEvent) []ActiveTunnel {
	// Track connect events by session ID
	connects := make(map[string]*audit.AuditEvent)

	// Process events in order
	for i := range events {
		e := &events[i]
		switch e.EventType {
		case audit.EventTypeConnect:
			connects[e.SessionID] = e
		case audit.EventTypeDisconnect, audit.EventTypeError:
			// Remove from active if we see a disconnect or error
			delete(connects, e.SessionID)
		}
	}

	// Convert remaining connects to active tunnels
	tunnels := make([]ActiveTunnel, 0, len(connects))
	now := time.Now()

	for _, e := range connects {
		uptime := now.Sub(e.Timestamp)
		tunnels = append(tunnels, ActiveTunnel{
			SessionID:   e.SessionID,
			ClusterName: e.ClusterName,
			Region:      e.Region,
			LocalPort:   e.LocalPort,
			RemoteHost:  e.RemoteHost,
			RemotePort:  e.RemotePort,
			BastionID:   e.BastionID,
			StartTime:   e.Timestamp,
			Uptime:      uptime,
			UptimeStr:   formatDuration(uptime),
		})
	}

	// Sort by start time (newest first)
	sort.Slice(tunnels, func(i, j int) bool {
		return tunnels[i].StartTime.After(tunnels[j].StartTime)
	})

	return tunnels
}

func outputJSON(tunnels []ActiveTunnel) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(tunnels)
}

func outputTable(tunnels []ActiveTunnel) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if statusVerbose {
		fmt.Fprintln(w, "CLUSTER\tLOCAL PORT\tREMOTE\tUPTIME\tSESSION ID\tSTARTED")
		for _, t := range tunnels {
			fmt.Fprintf(w, "%s\t:%d\t%s:%d\t%s\t%s\t%s\n",
				t.ClusterName,
				t.LocalPort,
				t.RemoteHost,
				t.RemotePort,
				t.UptimeStr,
				truncateSessionID(t.SessionID),
				t.StartTime.Local().Format("15:04:05"),
			)
		}
	} else {
		fmt.Fprintln(w, "CLUSTER\tLOCAL PORT\tREMOTE\tUPTIME")
		for _, t := range tunnels {
			fmt.Fprintf(w, "%s\t:%d\t%s:%d\t%s\n",
				t.ClusterName,
				t.LocalPort,
				t.RemoteHost,
				t.RemotePort,
				t.UptimeStr,
			)
		}
	}

	fmt.Fprintln(w)
	w.Flush()

	fmt.Printf("Total: %d active tunnel(s)\n", len(tunnels))
	return nil
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh%dm", hours, mins)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	return fmt.Sprintf("%dd%dh", days, hours)
}

// truncateSessionID shortens a session ID for display.
func truncateSessionID(id string) string {
	if len(id) <= 16 {
		return id
	}
	return id[:8] + "..." + id[len(id)-5:]
}
