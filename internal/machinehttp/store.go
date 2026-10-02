package machinehttp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

const maxExecutionEvents = 512

type executionRecord struct {
	execution   machine.Execution
	events      []machine.MachineEvent
	nextSeq     uint64
	subscribers map[chan machine.MachineEvent]struct{}
}

type executionStore struct {
	mu      sync.RWMutex
	records map[string]*executionRecord
}

func newExecutionStore() *executionStore {
	return &executionStore{records: map[string]*executionRecord{}}
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
	return nil
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
