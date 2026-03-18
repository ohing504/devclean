package ui

import (
	"fmt"
	"sync"
	"time"
)

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner displays an animated spinner with a message.
type Spinner struct {
	mu      sync.Mutex
	message string
	done    chan struct{}
	stopped bool
}

// NewSpinner creates and starts a new spinner with the given message.
func NewSpinner(message string) *Spinner {
	s := &Spinner{
		message: message,
		done:    make(chan struct{}),
	}
	go s.run()
	return s
}

// Update changes the spinner message.
func (s *Spinner) Update(message string) {
	s.mu.Lock()
	s.message = message
	s.mu.Unlock()
}

// Stop stops the spinner and clears the line.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()
	close(s.done)
	// Clear the spinner line
	fmt.Print("\r\033[K")
}

func (s *Spinner) run() {
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			msg := s.message
			s.mu.Unlock()
			fmt.Printf("\r\033[K%s %s", frames[i%len(frames)], msg)
			i++
		}
	}
}
