// Package recorder archives the raw signal.price.* tick stream to local
// append-only JSONL files so the replay engine (8.4) can re-drive the
// downstream pipeline offline.
//
// One file per calendar day: <dir>/2026-08-23.jsonl, one compact
// PriceTick JSON object per line.  Rotation happens lazily on the first
// write of a new day; no timers, no goroutines beyond the NATS consumer.
package recorder

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/models"
	"github.com/Nalin-Angrish/Navier-Stocks/src/pkg/nats"
)

// TickStore appends PriceTick records to daily JSONL files inside a
// directory.  It is safe for concurrent use.
type TickStore struct {
	dir string

	mu      sync.Mutex
	file    *os.File
	day     string // YYYY-MM-DD of the currently open file
	clockFn func() time.Time
}

// OpenTickStore returns a TickStore writing into dir.  The directory is
// created if missing.  No file is opened until the first Append.
func OpenTickStore(dir string) (*TickStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("recorder: create dir: %w", err)
	}
	return &TickStore{dir: dir, clockFn: time.Now}, nil
}

// SetClock overrides the wall clock used for rotation decisions.
// Intended for tests only.
func (s *TickStore) SetClock(fn func() time.Time) { s.clockFn = fn }

// Append writes one tick as a single JSON line, rotating the underlying
// file when the day changes.
func (s *TickStore) Append(tick models.PriceTick) error {
	line, err := json.Marshal(tick)
	if err != nil {
		return fmt.Errorf("recorder: marshal tick: %w", err)
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	day := s.clockFn().Format("2006-01-02")
	if s.file == nil || day != s.day {
		if err := s.rotateLocked(day); err != nil {
			return err
		}
	}
	if _, err := s.file.Write(line); err != nil {
		return fmt.Errorf("recorder: append tick: %w", err)
	}
	return nil
}

// rotateLocked closes the current file (if any) and opens the one for
// day.  Callers must hold s.mu.
func (s *TickStore) rotateLocked(day string) error {
	path := filepath.Join(s.dir, day+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("recorder: open %s: %w", path, err)
	}
	if s.file != nil {
		if err := s.file.Close(); err != nil {
			_ = f.Close()
			return fmt.Errorf("recorder: close %s: %w", s.file.Name(), err)
		}
	}
	s.file = f
	s.day = day
	return nil
}

// Close releases the current file handle.  Safe to call twice.
func (s *TickStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

// handlePrice decodes and stores one signal.price.* message.  Malformed
// messages are logged and dropped — a broken line must never stall the
// stream.
func (a *Analyst) handlePrice(m *nats.Msg) {
	var tick models.PriceTick
	if err := json.Unmarshal(m.Data, &tick); err != nil {
		log.Printf("[Recorder] dropping malformed tick (%v): %q", err, m.Data)
		return
	}
	if err := a.store.Append(tick); err != nil {
		log.Printf("[Recorder] append failed: %v", err)
	}
}
