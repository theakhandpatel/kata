package agent

import (
	"bufio"
	"io"
	"strings"
	"sync"
)

// StdinSteering collects messages typed while a run is in progress and hands them
// to the loop at turn boundaries.
//
// The point is the boundary. A message typed during a run must not be merged into
// the prompt that is already being answered; it belongs after the current turn, as
// a new message. That is what makes "wait, also check the tests" work without
// restarting the run.
type StdinSteering struct {
	scanner *bufio.Scanner

	mu    sync.Mutex
	queue []Message
	done  bool
}

// NewStdinSteering returns a queue that reads lines from r in the background.
//
// Reading starts when Start is called, not here, so a caller can wire everything
// up before any input is consumed.
func NewStdinSteering(r io.Reader) *StdinSteering {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
	return &StdinSteering{scanner: s}
}

// Start begins reading. It returns immediately.
func (s *StdinSteering) Start() {
	go func() {
		for s.scanner.Scan() {
			line := strings.TrimSpace(s.scanner.Text())
			if line == "" {
				continue
			}
			s.mu.Lock()
			s.queue = append(s.queue, Message{Role: RoleUser, Content: line})
			s.mu.Unlock()
		}
		s.mu.Lock()
		s.done = true
		s.mu.Unlock()
	}()
}

// Drain returns everything queued since the last call.
//
// It never blocks. A steering hook that blocks turns the loop into a program that
// waits for a human, which is the opposite of what this is for.
func (s *StdinSteering) Drain() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return nil
	}
	out := s.queue
	s.queue = nil
	return out
}

// Closed reports whether the input has ended.
func (s *StdinSteering) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}
