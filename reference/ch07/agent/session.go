package agent

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Session format version. Bump it when a record's meaning changes, so an old
// file is rejected instead of silently misread.
const SessionVersion = 1

// Record types written to a session file.
const (
	recordHeader  = "header"
	recordMessage = "message"
)

// sessionHeader is the first line of every session file.
type sessionHeader struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
	Created string `json:"created"`
}

// sessionMessage is one conversation entry.
type sessionMessage struct {
	Type    string  `json:"type"`
	Message Message `json:"message"`
}

// Recorder persists a conversation.
//
// The interface exists so that later chapters can change where history lives
// without touching the loop: a file now, a tree later, an in-memory recorder in a
// test.
type Recorder interface {
	// Load returns the messages recorded so far, in order.
	Load() ([]Message, error)
	// Append records one message.
	Append(Message) error
	// Close releases whatever the recorder holds.
	Close() error
}

// JSONLSession is an append-only session file: one JSON object per line.
//
// Append-only is the important property. Rewriting the file on every turn makes
// a crashed run lose the whole conversation, and it makes two concurrent readers
// see different histories.
type JSONLSession struct {
	path string
	file *os.File
}

var _ Recorder = (*JSONLSession)(nil)

// OpenSession opens a session file, creating it with a header when absent.
func OpenSession(path string) (*JSONLSession, error) {
	if path == "" {
		return nil, errors.New("session path is required")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	_, statErr := os.Stat(path)
	isNew := errors.Is(statErr, os.ErrNotExist)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	s := &JSONLSession{path: path, file: f}
	if isNew {
		header := sessionHeader{
			Type:    recordHeader,
			Version: SessionVersion,
			Created: time.Now().UTC().Format(time.RFC3339),
		}
		if err := s.writeLine(header); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	return s, nil
}

// Load reads every recorded message.
//
// The header is skipped. A malformed line is an error rather than a silent skip:
// a corrupt session should be visible before it produces a wrong conversation.
func (s *JSONLSession) Load() ([]Message, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}

		var probe struct {
			Type    string `json:"type"`
			Version int    `json:"version"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return nil, fmt.Errorf("session line %d: %w", line, err)
		}

		switch probe.Type {
		case recordHeader:
			if probe.Version > SessionVersion {
				return nil, fmt.Errorf("session version %d is newer than %d", probe.Version, SessionVersion)
			}
		case recordMessage:
			var rec sessionMessage
			if err := json.Unmarshal(raw, &rec); err != nil {
				return nil, fmt.Errorf("session line %d: %w", line, err)
			}
			out = append(out, rec.Message)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Append writes one message and flushes it.
func (s *JSONLSession) Append(m Message) error {
	return s.writeLine(sessionMessage{Type: recordMessage, Message: m})
}

// Close closes the underlying file.
func (s *JSONLSession) Close() error {
	if s.file == nil {
		return nil
	}
	return s.file.Close()
}

func (s *JSONLSession) writeLine(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = s.file.Write(b)
	return err
}
