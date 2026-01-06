package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/scotttball/tunatap/internal/audit"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View tunnel activity logs",
	Long: `Stream or view recent tunnel activity from audit logs.

Examples:
  tunatap logs                  # Show last 20 events
  tunatap logs -n 50            # Show last 50 events
  tunatap logs -f               # Follow new events (like tail -f)
  tunatap logs --cluster my-k8s # Filter by cluster name
`,
	RunE: runLogs,
}

var (
	logsFollow  bool
	logsLines   int
	logsCluster string
	logsJSON    bool
)

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow log output (like tail -f)")
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 20, "number of lines to show")
	logsCmd.Flags().StringVarP(&logsCluster, "cluster", "c", "", "filter by cluster name")
	logsCmd.Flags().BoolVar(&logsJSON, "json", false, "output raw JSON events")
}

func runLogs(cmd *cobra.Command, args []string) error {
	logDir := audit.DefaultLogDir()

	if logsFollow {
		return followLogs(logDir)
	}

	return showRecentLogs(logDir)
}

func showRecentLogs(logDir string) error {
	// Query recent events
	q := audit.Query{
		ClusterName: logsCluster,
		Limit:       logsLines,
	}

	events, err := audit.QueryLogs(logDir, q)
	if err != nil {
		return fmt.Errorf("failed to query audit logs: %w", err)
	}

	if len(events) == 0 {
		fmt.Println("No log events found")
		return nil
	}

	for i := range events {
		printEvent(&events[i])
	}

	return nil
}

func followLogs(logDir string) error {
	// Find today's log file
	logFile := filepath.Join(logDir, fmt.Sprintf("audit-%s.jsonl", time.Now().Format("2006-01-02")))

	// Check if file exists, if not wait for it
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		fmt.Printf("Waiting for log file: %s\n", logFile)
	}

	// Open file
	file, err := os.Open(logFile)
	if err != nil {
		// File doesn't exist yet, create directory and wait
		if os.IsNotExist(err) {
			if err := os.MkdirAll(logDir, 0o750); err != nil {
				return fmt.Errorf("failed to create log directory: %w", err)
			}
			// Wait for file to appear
			return waitAndFollow(logFile)
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Seek to end to start following
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}

	fmt.Printf("Following %s (Ctrl+C to stop)\n\n", logFile)

	return tailFile(file)
}

func waitAndFollow(logFile string) error {
	// Poll for file to appear
	fmt.Printf("Waiting for log file: %s\n", logFile)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute)

	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(logFile); err == nil {
				return openAndTailFile(logFile)
			}
		case <-timeout:
			return fmt.Errorf("timed out waiting for log file")
		}
	}
}

func openAndTailFile(logFile string) error {
	file, err := os.Open(logFile)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()
	fmt.Printf("Following %s (Ctrl+C to stop)\n\n", logFile)
	return tailFile(file)
}

func tailFile(file *os.File) error {
	reader := bufio.NewReader(file)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// No more data, wait and try again
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return fmt.Errorf("error reading log file: %w", err)
		}

		// Parse and print the event
		var event audit.AuditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue // Skip malformed lines
		}

		// Apply cluster filter
		if logsCluster != "" && event.ClusterName != logsCluster {
			continue
		}

		printEvent(&event)
	}
}

func printEvent(e *audit.AuditEvent) {
	if logsJSON {
		data, _ := json.Marshal(e)
		fmt.Println(string(data))
	} else {
		fmt.Println(audit.FormatEvent(e))
	}
}

// GetLogFiles returns all audit log files sorted by date (newest first).
func GetLogFiles(logDir string) ([]string, error) {
	pattern := filepath.Join(logDir, "audit-*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	// Sort by filename (which includes date) in reverse order
	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	return files, nil
}
