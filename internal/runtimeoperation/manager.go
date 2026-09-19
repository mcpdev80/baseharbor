package runtimeoperation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StateSucceeded State = "succeeded"
	StateFailed    State = "failed"
)

type Request struct {
	Application    string         `json:"application"`
	Capability     string         `json:"capability"`
	Operation      string         `json:"operation"`
	ResourceName   string         `json:"resource_name"`
	IdempotencyKey string         `json:"idempotency_key"`
	Parameters     map[string]any `json:"parameters,omitempty"`
}

type Result struct {
	ResourceID string         `json:"resource_id,omitempty"`
	Binding    map[string]any `json:"binding,omitempty"`
}

type Operation struct {
	ID        string    `json:"id"`
	Request   Request   `json:"request"`
	State     State     `json:"state"`
	Result    Result    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Executor interface {
	Execute(context.Context, Request) (Result, error)
}

type ExecutorFunc func(context.Context, Request) (Result, error)

func (f ExecutorFunc) Execute(ctx context.Context, request Request) (Result, error) {
	return f(ctx, request)
}

var ErrNotFound = errors.New("runtime operation not found")

type Manager struct {
	mu        sync.Mutex
	dir       string
	executors map[string]Executor
	ops       map[string]Operation
	keys      map[string]string
}

func New(dir string, executors map[string]Executor) (*Manager, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("runtime operation state directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime operation state directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("protect runtime operation state directory: %w", err)
	}
	m := &Manager{
		dir:       dir,
		executors: map[string]Executor{},
		ops:       map[string]Operation{},
		keys:      map[string]string{},
	}
	for key, executor := range executors {
		if executor != nil {
			m.executors[strings.TrimSpace(key)] = executor
		}
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Submit(ctx context.Context, request Request) (Operation, bool, error) {
	if err := validateRequest(request); err != nil {
		return Operation{}, false, err
	}
	key := executorKey(request.Capability, request.Operation)
	executor := m.executors[key]
	if executor == nil {
		return Operation{}, false, fmt.Errorf("unsupported capability operation %s %s", request.Capability, request.Operation)
	}

	m.mu.Lock()
	idem := idempotencyKey(request)
	if existingID := m.keys[idem]; existingID != "" {
		existing := m.ops[existingID]
		m.mu.Unlock()
		return existing, true, nil
	}
	id, err := newID()
	if err != nil {
		m.mu.Unlock()
		return Operation{}, false, err
	}
	now := time.Now().UTC()
	op := Operation{
		ID: id, Request: cloneRequest(request), State: StatePending,
		CreatedAt: now, UpdatedAt: now,
	}
	m.ops[id] = op
	m.keys[idem] = id
	if err := m.persistLocked(); err != nil {
		delete(m.ops, id)
		delete(m.keys, idem)
		m.mu.Unlock()
		return Operation{}, false, err
	}
	m.mu.Unlock()

	go m.execute(context.WithoutCancel(ctx), id, executor)
	return op, false, nil
}

func (m *Manager) Get(id string) (Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.ops[strings.TrimSpace(id)]
	if !ok {
		return Operation{}, ErrNotFound
	}
	return cloneOperation(op), nil
}

func (m *Manager) FindResource(resourceID string) (Request, error) {
	resourceID = strings.TrimSpace(resourceID)
	if resourceID == "" {
		return Request{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, op := range m.ops {
		if op.Request.Operation != "runtime.create" || op.State != StateSucceeded || op.Result.ResourceID != resourceID {
			continue
		}
		return cloneRequest(op.Request), nil
	}
	return Request{}, ErrNotFound
}

func (m *Manager) Resume(ctx context.Context) error {
	m.mu.Lock()
	var pending []string
	for id, op := range m.ops {
		if op.State == StatePending || op.State == StateRunning {
			op.State = StatePending
			op.Error = ""
			op.UpdatedAt = time.Now().UTC()
			m.ops[id] = op
			pending = append(pending, id)
		}
	}
	if err := m.persistLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	sort.Strings(pending)
	type queued struct {
		id       string
		executor Executor
	}
	var jobs []queued
	for _, id := range pending {
		op := m.ops[id]
		executor := m.executors[executorKey(op.Request.Capability, op.Request.Operation)]
		if executor == nil {
			op.State = StateFailed
			op.Error = "unsupported capability operation after restart"
			op.UpdatedAt = time.Now().UTC()
			m.ops[id] = op
			continue
		}
		jobs = append(jobs, queued{id: id, executor: executor})
	}
	if err := m.persistLocked(); err != nil {
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()
	for _, job := range jobs {
		go m.execute(context.WithoutCancel(ctx), job.id, job.executor)
	}
	return nil
}

func (m *Manager) execute(ctx context.Context, id string, executor Executor) {
	m.update(id, func(op *Operation) {
		op.State = StateRunning
		op.Error = ""
	})
	m.mu.Lock()
	request := cloneRequest(m.ops[id].Request)
	m.mu.Unlock()

	result, err := executor.Execute(ctx, request)
	if err != nil {
		m.update(id, func(op *Operation) {
			op.State = StateFailed
			op.Error = safeError(err)
		})
		return
	}
	m.update(id, func(op *Operation) {
		op.State = StateSucceeded
		op.Result = cloneResult(result)
		op.Error = ""
	})
}

func (m *Manager) update(id string, change func(*Operation)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.ops[id]
	if !ok {
		return
	}
	change(&op)
	op.UpdatedAt = time.Now().UTC()
	m.ops[id] = op
	_ = m.persistLocked()
}

func (m *Manager) load() error {
	path := filepath.Join(m.dir, "operations.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read runtime operations: %w", err)
	}
	var items []Operation
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("decode runtime operations: %w", err)
	}
	for _, op := range items {
		if err := validateRequest(op.Request); err != nil || strings.TrimSpace(op.ID) == "" {
			return errors.New("runtime operation state is invalid")
		}
		m.ops[op.ID] = cloneOperation(op)
		m.keys[idempotencyKey(op.Request)] = op.ID
	}
	return nil
}

func (m *Manager) persistLocked() error {
	items := make([]Operation, 0, len(m.ops))
	for _, op := range m.ops {
		items = append(items, cloneOperation(op))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(m.dir, "operations.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write runtime operation state: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit runtime operation state: %w", err)
	}
	return nil
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.Application) == "" || strings.TrimSpace(request.Capability) == "" ||
		strings.TrimSpace(request.Operation) == "" || strings.TrimSpace(request.ResourceName) == "" ||
		strings.TrimSpace(request.IdempotencyKey) == "" {
		return errors.New("runtime operation request is incomplete")
	}
	if len(request.IdempotencyKey) > 200 {
		return errors.New("idempotency key exceeds 200 characters")
	}
	return nil
}

func executorKey(capability, operation string) string {
	return strings.TrimSpace(capability) + "\x00" + strings.TrimSpace(operation)
}

func idempotencyKey(request Request) string {
	return request.Application + "\x00" + request.Capability + "\x00" + request.Operation + "\x00" + request.IdempotencyKey
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate runtime operation id: %w", err)
	}
	return "op-" + hex.EncodeToString(value[:]), nil
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "runtime operation failed"
	}
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}

func cloneRequest(request Request) Request {
	cloned := request
	if request.Parameters != nil {
		data, _ := json.Marshal(request.Parameters)
		_ = json.Unmarshal(data, &cloned.Parameters)
	}
	return cloned
}

func cloneResult(result Result) Result {
	cloned := result
	if result.Binding != nil {
		data, _ := json.Marshal(result.Binding)
		_ = json.Unmarshal(data, &cloned.Binding)
	}
	return cloned
}

func cloneOperation(operation Operation) Operation {
	cloned := operation
	cloned.Request = cloneRequest(operation.Request)
	cloned.Result = cloneResult(operation.Result)
	return cloned
}
