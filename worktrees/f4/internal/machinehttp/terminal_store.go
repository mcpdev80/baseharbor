package machinehttp

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/runtime/terminal"
)

type terminalRecord struct {
	descriptor    machine.StreamDescriptor
	request       machine.StreamRequest
	ctx           context.Context
	cancel        context.CancelFunc
	session       terminal.Session
	mu            sync.Mutex
	inputSequence uint64
	attached      bool
}

type terminalStore struct {
	mu      sync.Mutex
	records map[string]*terminalRecord
}

func newTerminalStore() *terminalStore { return &terminalStore{records: map[string]*terminalRecord{}} }

func (s *terminalStore) reserve(record *terminalRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) >= 16 {
		return errors.New("terminal capacity exhausted")
	}
	actorCount := 0
	for _, existing := range s.records {
		if existing.descriptor.Actor.Subject == record.descriptor.Actor.Subject && existing.descriptor.Actor.Issuer == record.descriptor.Actor.Issuer {
			actorCount++
		}
	}
	if actorCount >= 4 {
		return errors.New("operator terminal capacity exhausted")
	}
	s.records[record.descriptor.StreamID] = record
	return nil
}

func (s *terminalStore) get(id string) *terminalRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.records[id]
}

func (s *terminalStore) remove(id string) {
	s.mu.Lock()
	delete(s.records, id)
	s.mu.Unlock()
}

func (r *terminalRecord) startLifetime(store *terminalStore, auditClose func()) {
	// Creation does not entitle a client to leave a process unattended.
	timer := time.AfterFunc(10*time.Second, func() {
		r.mu.Lock()
		attached := r.attached
		r.mu.Unlock()
		if !attached {
			r.cancel()
		}
	})
	context.AfterFunc(r.ctx, func() {
		timer.Stop()
		_ = r.session.Close()
		store.remove(r.descriptor.StreamID)
		auditClose()
	})
}
