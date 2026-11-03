package machinehttp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

const maxExecutionEvents = 512

// journalDir returns the directory that persists execution-journal records under
// a state-root. An empty state-root leaves the store purely in-memory (existing
// behavior); the production caller supplies the runtime state-root so the
// journal survives process restarts.
func journalDir(stateRoot string) string {
	if stateRoot == "" {
		return ""
	}
	return filepath.Join(stateRoot, "journal")
}

type executionRecord struct {
	execution   machine.Execution
	events      []machine.MachineEvent
	nextSeq     uint64
	subscribers map[chan machine.MachineEvent]struct{}
}

type executionStore struct {
	mu          sync.RWMutex
	records     map[string]*executionRecord
	journalPath string
}

func newExecutionStore() *executionStore {
	return &executionStore{records: map[string]*executionRecord{}}
}

// newPersistedExecutionStore loads any existing journal from stateRoot and
// persists every subsequent mutation to it. stateRoot == "" keeps the store
// in-memory. A corrupt journal line fails closed (never silently skipped).
func newPersistedExecutionStore(stateRoot string) (*executionStore, error) {
	store := &executionStore{records: map[string]*executionRecord{}}
	dir := journalDir(stateRoot)
	if dir == "" {
		return store, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create execution journal dir %q: %w", dir, err)
	}
	store.journalPath = filepath.Join(dir, "journal.jsonl")
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func newExecutionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "exec_" + hex.EncodeToString(raw[:]), nil
}

func (s *executionStore) create(operationID string, actor machine.ActorRef, ctx machine.OperationContext) (machine.Execution, error) {
	id, err := newExecutionID()
	if err != nil {
		return machine.Execution{}, err
	}
	execution := machine.Execution{
		ContractVersion: machine.ExecutionContractVersion,
		ExecutionID:     id,
		OperationID:     operationID,
		Actor:           actor,
		Context:         ctx,
		State:           machine.ExecutionPending,
	}
	s.mu.Lock()
	s.records[id] = &executionRecord{
		execution:   execution,
		subscribers: map[chan machine.MachineEvent]struct{}{},
	}
	s.mu.Unlock()
	return execution, nil
}

func (s *executionStore) get(id string) (machine.Execution, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.records[id]
	if !ok {
		return machine.Execution{}, false
	}
	return record.execution, true
}

func (s *executionStore) start(id string) error {
	now := time.Now().UTC()
	return s.update(id, func(record *executionRecord) {
		record.execution.State = machine.ExecutionRunning
		record.execution.StartedAt = &now
		s.emitLocked(record, machine.MachineEvent{
			Kind:        machine.EventOperationStarted,
			ExecutionID: id,
			OperationID: record.execution.OperationID,
			Actor:       &record.execution.Actor,
			Context:     &record.execution.Context,
			State:       machine.ExecutionRunning,
		})
	})
}

func (s *executionStore) progress(id string, progress machine.OperationProgress) error {
	copy := progress
	return s.update(id, func(record *executionRecord) {
		record.execution.Progress = &copy
		s.emitLocked(record, machine.MachineEvent{
			Kind:        machine.EventOperationProgress,
			ExecutionID: id,
			OperationID: record.execution.OperationID,
			State:       record.execution.State,
			Progress:    &copy,
		})
	})
}

func (s *executionStore) succeed(id string, result []byte) error {
	now := time.Now().UTC()
	resultCopy := append([]byte(nil), result...)
	return s.update(id, func(record *executionRecord) {
		record.execution.State = machine.ExecutionSucceeded
		record.execution.FinishedAt = &now
		record.execution.Result = resultCopy
		record.execution.Error = nil
		s.emitLocked(record, machine.MachineEvent{
			Kind:        machine.EventOperationSucceeded,
			ExecutionID: id,
			OperationID: record.execution.OperationID,
			State:       machine.ExecutionSucceeded,
			Result:      resultCopy,
		})
	})
}

func (s *executionStore) fail(id string, err error) error {
	now := time.Now().UTC()
	classified := machine.Classify(err)
	return s.update(id, func(record *executionRecord) {
		record.execution.State = machine.ExecutionFailed
		record.execution.FinishedAt = &now
		record.execution.Error = classified
		s.emitLocked(record, machine.MachineEvent{
			Kind:        machine.EventOperationFailed,
			ExecutionID: id,
			OperationID: record.execution.OperationID,
			State:       machine.ExecutionFailed,
			Error:       classified,
		})
	})
}

func (s *executionStore) update(id string, fn func(*executionRecord)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return errors.New("execution not found")
	}
	fn(record)
	if err := s.appendJournalLocked(record); err != nil {
		return err
	}
	return nil
}

// appendJournalLocked writes the record's current state as one append-only
// journal line. Callers must hold s.mu. A persistence error is surfaced to the
// caller (fail closed on write) rather than swallowed.
func (s *executionStore) appendJournalLocked(record *executionRecord) error {
	if s.journalPath == "" {
		return nil
	}
	line, err := json.Marshal(struct {
		ExecutionID string                     `json:"execution_id"`
		State       machine.ExecutionState     `json:"state"`
		StartedAt   *time.Time                 `json:"started_at,omitempty"`
		FinishedAt  *time.Time                 `json:"finished_at,omitempty"`
		Progress    *machine.OperationProgress `json:"progress,omitempty"`
		Result      json.RawMessage            `json:"result,omitempty"`
		Error       *machine.Error             `json:"error,omitempty"`
	}{
		ExecutionID: record.execution.ExecutionID,
		State:       record.execution.State,
		StartedAt:   record.execution.StartedAt,
		FinishedAt:  record.execution.FinishedAt,
		Progress:    record.execution.Progress,
		Result:      record.execution.Result,
		Error:       record.execution.Error,
	})
	if err != nil {
		return fmt.Errorf("marshal execution journal line: %w", err)
	}
	f, err := os.OpenFile(s.journalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open execution journal %q: %w", s.journalPath, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append execution journal line: %w", err)
	}
	return nil
}

// load reconstructs in-memory records from the journal. It fails closed on a
// corrupt line: the first unparseable record is an error, never a silent skip.
func (s *executionStore) load() error {
	data, err := os.ReadFile(s.journalPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read execution journal %q: %w", s.journalPath, err)
	}
	for i, raw := range splitLines(data) {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var line struct {
			ExecutionID string                     `json:"execution_id"`
			State       machine.ExecutionState     `json:"state"`
			StartedAt   *time.Time                 `json:"started_at,omitempty"`
			FinishedAt  *time.Time                 `json:"finished_at,omitempty"`
			Progress    *machine.OperationProgress `json:"progress,omitempty"`
			Result      json.RawMessage            `json:"result,omitempty"`
			Error       *machine.Error             `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			return fmt.Errorf("corrupt execution journal line %d in %q: %w", i+1, s.journalPath, err)
		}
		if line.ExecutionID == "" {
			return fmt.Errorf("corrupt execution journal line %d in %q: missing execution_id", i+1, s.journalPath)
		}
		record := &executionRecord{
			execution: machine.Execution{
				ContractVersion: machine.ExecutionContractVersion,
				ExecutionID:     line.ExecutionID,
				State:           line.State,
				StartedAt:       line.StartedAt,
				FinishedAt:      line.FinishedAt,
				Progress:        line.Progress,
				Result:          line.Result,
				Error:           line.Error,
			},
			nextSeq:     0,
			subscribers: map[chan machine.MachineEvent]struct{}{},
		}
		s.records[line.ExecutionID] = record
	}
	return nil
}

// splitLines splits journal bytes on newlines, tolerating a trailing newline.
func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func (s *executionStore) emitLocked(record *executionRecord, event machine.MachineEvent) {
	record.nextSeq++
	event.ContractVersion = machine.ExecutionContractVersion
	event.Sequence = record.nextSeq
	event.OccurredAt = time.Now().UTC()
	record.events = append(record.events, event)
	if len(record.events) > maxExecutionEvents {
		record.events = append([]machine.MachineEvent(nil), record.events[len(record.events)-maxExecutionEvents:]...)
	}
	for subscriber := range record.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (s *executionStore) subscribe(id string) ([]machine.MachineEvent, <-chan machine.MachineEvent, func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return nil, nil, nil, false
	}
	history := append([]machine.MachineEvent(nil), record.events...)
	ch := make(chan machine.MachineEvent, 32)
	record.subscribers[ch] = struct{}{}
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if current, exists := s.records[id]; exists {
			if _, subscribed := current.subscribers[ch]; subscribed {
				delete(current.subscribers, ch)
				close(ch)
			}
		}
	}
	return history, ch, cancel, true
}
