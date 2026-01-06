package ui

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Spinner displays a progress indicator during long-running operations.
type Spinner struct {
	message   string
	done      chan struct{}
	wg        sync.WaitGroup
	isRunning bool
	mu        sync.Mutex
}

// Default spinner frames (dots animation)
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		done:    make(chan struct{}),
	}
}

// Start begins the spinner animation.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.done = make(chan struct{})
	s.mu.Unlock()

	// Only show spinner if stdout is a terminal
	if !IsTerminal() {
		fmt.Printf("%s...\n", s.message)
		return
	}

	s.wg.Add(1)
	go s.animate()
}

// Stop stops the spinner and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = false
	s.mu.Unlock()

	close(s.done)
	s.wg.Wait()

	// Clear the line
	if IsTerminal() {
		fmt.Print("\r\033[K")
	}
}

// StopWithMessage stops the spinner and shows a final message.
func (s *Spinner) StopWithMessage(message string) {
	s.Stop()
	fmt.Println(message)
}

// StopWithSuccess stops the spinner and shows a success message.
func (s *Spinner) StopWithSuccess(message string) {
	s.Stop()
	if IsTerminal() {
		fmt.Printf("✓ %s\n", message)
	} else {
		fmt.Printf("[OK] %s\n", message)
	}
}

// StopWithError stops the spinner and shows an error message.
func (s *Spinner) StopWithError(message string) {
	s.Stop()
	if IsTerminal() {
		fmt.Printf("✗ %s\n", message)
	} else {
		fmt.Printf("[ERROR] %s\n", message)
	}
}

// UpdateMessage changes the spinner message while running.
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	s.message = message
	s.mu.Unlock()
}

func (s *Spinner) animate() {
	defer s.wg.Done()

	frames := spinnerFrames
	frameIndex := 0
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			message := s.message
			s.mu.Unlock()

			// Print spinner frame and message
			fmt.Printf("\r%s %s", frames[frameIndex], message)
			frameIndex = (frameIndex + 1) % len(frames)
		}
	}
}

// IsTerminal checks if stdout is connected to a terminal.
func IsTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// RunWithSpinner executes a function while showing a spinner.
// Returns the error from the function.
func RunWithSpinner(message string, fn func() error) error {
	spinner := NewSpinner(message)
	spinner.Start()

	err := fn()

	if err != nil {
		spinner.StopWithError(fmt.Sprintf("%s: %v", message, err))
	} else {
		spinner.StopWithSuccess(message)
	}

	return err
}

// RunWithSpinnerResult executes a function that returns a value while showing a spinner.
func RunWithSpinnerResult[T any](message string, fn func() (T, error)) (T, error) {
	spinner := NewSpinner(message)
	spinner.Start()

	result, err := fn()

	if err != nil {
		spinner.StopWithError(fmt.Sprintf("%s: %v", message, err))
	} else {
		spinner.StopWithSuccess(message)
	}

	return result, err
}

// Progress represents a progress indicator for known-length operations.
type Progress struct {
	message string
	current int
	total   int
	mu      sync.Mutex
}

// NewProgress creates a new progress indicator.
func NewProgress(message string, total int) *Progress {
	return &Progress{
		message: message,
		total:   total,
	}
}

// Update updates the progress.
func (p *Progress) Update(current int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = current

	if !IsTerminal() {
		return
	}

	percent := float64(current) / float64(p.total) * 100
	fmt.Printf("\r%s: %.0f%% (%d/%d)", p.message, percent, current, p.total)
}

// Done completes the progress and moves to next line.
func (p *Progress) Done() {
	if IsTerminal() {
		fmt.Println()
	}
}
