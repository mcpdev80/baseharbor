package runtimeexplorer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

// ConnectorBackend projects the existing Runtime Explorer primitives through
// canonical typed transport. Service still evaluates ownership and safety.
type ConnectorTransport interface {
	LiveCapabilities(targetenrollment.Scope) (targetsession.Capabilities, error)
	Dispatch(context.Context, targetenrollment.Scope, targetsession.Request) (targetsession.Response, error)
}

type ConnectorBackend struct {
	pool  ConnectorTransport
	scope targetenrollment.Scope
}

func NewConnectorBackend(pool ConnectorTransport, scope targetenrollment.Scope) (*ConnectorBackend, error) {
	if pool == nil || scope.Validate() != nil {
		return nil, targetsession.ErrUnavailable
	}
	if _, err := pool.LiveCapabilities(scope); err != nil {
		return nil, err
	}
	return &ConnectorBackend{pool, scope}, nil
}

func (b *ConnectorBackend) Kind() runtimecontract.ProviderKind {
	return runtimecontract.ProviderKind(b.scope.Runtime)
}

func (b *ConnectorBackend) RuntimeExplorerCapabilities(ctx context.Context) ([]Capability, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	negotiated, err := b.pool.LiveCapabilities(b.scope)
	if err != nil {
		return nil, err
	}
	available := map[string]bool{}
	for _, capability := range negotiated.Capabilities {
		available[capability.Name] = capability.Available
	}
	var result []Capability
	if available["runtime.resource.list"] && available["runtime.resource.inspect"] {
		result = append(result, CapabilityResourceInspect)
	}
	if available["runtime.container.start"] && available["runtime.container.stop"] && available["runtime.container.restart"] {
		result = append(result, CapabilityContainerLifecycle)
	}
	if available["runtime.exec"] {
		result = append(result, CapabilityContainerExec)
	}
	// Follow/interactive transport is not advertised until its adapter exists.
	return result, nil
}

func (b *ConnectorBackend) invoke(ctx context.Context, operation string, payload any, destination any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	id := hex.EncodeToString(nonce[:])
	correlation := machine.ExecutionCorrelation(ctx)
	if correlation == "" {
		correlation = id
	}
	now := time.Now().UTC()
	deadline := now.Add(time.Minute)
	if bound, ok := ctx.Deadline(); ok && bound.Before(deadline) {
		deadline = bound.UTC()
	}
	response, err := b.pool.Dispatch(ctx, b.scope, targetsession.Request{ContractVersion: "baseharbor.target-access/v1", ProtocolVersion: "1", RequestID: id, CorrelationID: correlation, TargetID: b.scope.TargetID, Operation: operation, IssuedAt: now, DeadlineAt: deadline, Payload: data})
	if err != nil {
		return machine.NewError(machine.ErrorCapabilityMissing, "Authenticated remote runtime operation is unavailable.", "Reconnect the selected Target and reconcile observed state before a new mutation.", true)
	}
	if !response.Success {
		return machine.NewError(machine.ErrorRuntimeUnavailable, "The admitted remote runtime operation failed.", "Inspect the selected Target evidence and reconcile observed state before retrying.", false)
	}
	if destination == nil {
		return nil
	}
	if json.Unmarshal(response.Result, destination) != nil {
		return errors.New("invalid remote runtime result")
	}
	return nil
}

func (b *ConnectorBackend) ListRuntimeContainers(ctx context.Context) ([]runtimecontract.RuntimeContainer, error) {
	var wire []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		State   string `json:"state"`
		Project string `json:"compose_project"`
		Service string `json:"compose_service"`
	}
	if err := b.invoke(ctx, "runtime.resource.list", struct{}{}, &wire); err != nil {
		return nil, err
	}
	result := make([]runtimecontract.RuntimeContainer, 0, len(wire))
	seen := map[string]bool{}
	for _, item := range wire {
		if item.ID == "" || item.Name == "" || seen[item.ID] {
			return nil, errors.New("ambiguous remote container inventory")
		}
		seen[item.ID] = true
		result = append(result, runtimecontract.RuntimeContainer{ID: item.ID, Name: item.Name, Project: item.Project, Service: item.Service, State: item.State, Running: item.State == "running"})
	}
	return result, nil
}

func (b *ConnectorBackend) ContainerLogs(ctx context.Context, id string, since *time.Time, tail int, follow bool) (io.ReadCloser, error) {
	if follow {
		return nil, machine.NewError(machine.ErrorCapabilityMissing, "Remote follow stream is not bound.", "Use bounded log retrieval until live stream qualification completes.", false)
	}
	if tail <= 0 {
		tail = 200
	}
	if tail > 10000 {
		return nil, errors.New("remote log tail exceeds bound")
	}
	payload := map[string]any{"resource_id": id, "tail": tail}
	if since != nil {
		payload["since"] = since.UTC().Format(time.RFC3339Nano)
	}
	var output string
	if err := b.invoke(ctx, "runtime.logs.read", payload, &output); err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(output)), nil
}

func (b *ConnectorBackend) OperateContainer(ctx context.Context, id string, operation Operation, argv []string) (string, error) {
	payload := map[string]any{"resource_id": id}
	switch operation {
	case OperationStart, OperationStop, OperationRestart:
		if len(argv) != 0 {
			return "", errors.New("container lifecycle cannot carry argv")
		}
		return "", b.invoke(ctx, "runtime.container."+string(operation), payload, nil)
	case OperationExec:
		payload["argv"] = argv
		payload["timeout_seconds"] = 60
		var result struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode int    `json:"exit_code"`
		}
		if err := b.invoke(ctx, "runtime.exec", payload, &result); err != nil {
			return "", err
		}
		if result.ExitCode != 0 {
			return "", errors.New("remote container command exited unsuccessfully")
		}
		return result.Stdout, nil
	default:
		return "", errors.New("unsupported remote container operation")
	}
}
