package runtimeexplorer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/contracts"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
)

type remoteTransport struct {
	calls  []targetsession.Request
	failed bool
	result json.RawMessage
	scope  targetenrollment.Scope
}

func (r *remoteTransport) LiveCapabilities(scope targetenrollment.Scope) (targetsession.Capabilities, error) {
	if scope != r.scope {
		return targetsession.Capabilities{}, targetsession.ErrUnavailable
	}
	return targetsession.Capabilities{Capabilities: []targetsession.Capability{{Name: "runtime.resource.list", Available: true}, {Name: "runtime.resource.inspect", Available: true}, {Name: "runtime.container.start", Available: true}, {Name: "runtime.container.stop", Available: true}, {Name: "runtime.container.restart", Available: true}, {Name: "runtime.exec", Available: true}}}, nil
}
func (r *remoteTransport) Dispatch(_ context.Context, scope targetenrollment.Scope, request targetsession.Request) (targetsession.Response, error) {
	if scope != r.scope {
		return targetsession.Response{}, targetsession.ErrUnavailable
	}
	data, _ := json.Marshal(request)
	if err := contracts.ValidateTargetAccessRecord("request", data); err != nil {
		return targetsession.Response{}, err
	}
	r.calls = append(r.calls, request)
	if r.failed {
		return targetsession.Response{}, targetsession.ErrUnavailable
	}
	return targetsession.Response{Success: true, Result: r.result}, nil
}
func newRemoteBackend(t *testing.T) (*ConnectorBackend, *remoteTransport) {
	t.Helper()
	scope := targetenrollment.Scope{TenantID: "11111111-1111-4111-8111-111111111111", TargetID: "lab", NodeID: "node-a", Runtime: "docker"}
	transport := &remoteTransport{scope: scope, result: json.RawMessage(`null`)}
	backend, err := NewConnectorBackend(transport, scope)
	if err != nil {
		t.Fatal(err)
	}
	return backend, transport
}

func TestRemoteExplorerMapsObservedContainerAndCoreOwnershipMetadata(t *testing.T) {
	b, r := newRemoteBackend(t)
	r.result = json.RawMessage(`[{"id":"container-a","name":"api","state":"running","compose_project":"owned-project","compose_service":"api"}]`)
	items, err := b.ListRuntimeContainers(context.Background())
	if err != nil || len(items) != 1 || items[0].Project != "owned-project" || items[0].Service != "api" || !items[0].Running {
		t.Fatal(items, err)
	}
	if len(r.calls) != 1 || r.calls[0].Operation != "runtime.resource.list" {
		t.Fatal(r.calls)
	}
	r.result = json.RawMessage(`[{"id":"container-a","name":"api"},{"id":"container-a","name":"foreign"}]`)
	if _, err := b.ListRuntimeContainers(context.Background()); err == nil {
		t.Fatal("duplicate identity accepted")
	}
}

func TestRemoteExplorerUsesBoundedTypedExecAndDoesNotReplayFailure(t *testing.T) {
	b, r := newRemoteBackend(t)
	r.result = json.RawMessage(`{"stdout":"hello","exit_code":0}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = machine.WithExecutionCorrelation(ctx, "execution-from-core")
	output, err := b.OperateContainer(ctx, "container-a", OperationExec, []string{"printf", "hello"})
	if err != nil || output != "hello" || len(r.calls) != 1 || r.calls[0].Operation != "runtime.exec" {
		t.Fatal(output, err, r.calls)
	}
	if r.calls[0].CorrelationID != "execution-from-core" {
		t.Fatal("Core execution correlation lost", r.calls[0])
	}
	deadline, _ := ctx.Deadline()
	if r.calls[0].DeadlineAt.After(deadline) {
		t.Fatal("transport expanded caller deadline")
	}
	r.failed = true
	if _, err := b.OperateContainer(ctx, "container-a", OperationStop, nil); err == nil || len(r.calls) != 2 {
		t.Fatal("remote error swallowed or replayed", err, r.calls)
	}
	if _, err := b.OperateContainer(ctx, "container-a", OperationStart, []string{"shell"}); err == nil || len(r.calls) != 2 {
		t.Fatal("lifecycle argv dispatched")
	}
}

func TestRemoteExplorerDoesNotAdvertiseUnboundStreamsOrTerminal(t *testing.T) {
	b, r := newRemoteBackend(t)
	service, err := NewService(b, "lab", nil)
	if err != nil {
		t.Fatal(err)
	}
	caps, err := service.Capabilities(context.Background(), "lab")
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range caps.Capabilities {
		if capability == CapabilityLogs || capability == CapabilityContainerTerminal {
			t.Fatal("unbound live stream advertised", capability)
		}
	}
	if _, err := service.Capabilities(context.Background(), "foreign"); err == nil {
		t.Fatal("foreign capability context accepted")
	}
	if _, err := b.ContainerLogs(context.Background(), "container-a", nil, 10, true); err == nil || len(r.calls) != 0 {
		t.Fatal("follow request downgraded to bounded logs")
	}
	r.result = json.RawMessage(`"bounded output"`)
	logs, err := b.ContainerLogs(context.Background(), "container-a", nil, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(logs)
	if string(data) != "bounded output" {
		t.Fatal(string(data))
	}
	if _, err := NewConnectorBackend(nil, b.scope); !errors.Is(err, targetsession.ErrUnavailable) {
		t.Fatal("missing remote fell back", err)
	}
}
