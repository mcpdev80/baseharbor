package runtimeexplorer

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
	"github.com/mcpdev80/baseharbor/internal/runtime/terminal"
)

type terminalOwnershipBackend struct {
	fakeBackend
	opens int
}

func (b *terminalOwnershipBackend) ContainerTerminal(context.Context, string, []string, int, int) (terminal.Session, error) {
	b.opens++
	return nil, errors.New("test backend admission reached")
}

func TestTerminalOwnershipEnvironmentAndProviderBeforeRuntimeAdmission(t *testing.T) {
	backend := &terminalOwnershipBackend{fakeBackend: fakeBackend{containers: []runtimecontract.RuntimeContainer{{ID: "owned", Project: "bh-demo"}, {ID: "foreign"}}}}
	service, err := NewService(backend, "local", fakeResolver{})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ id, environment, provider string }{{"foreign", "dev", "docker"}, {"owned", "prod", "docker"}, {"owned", "dev", "podman"}} {
		_, err := service.Terminal(context.Background(), OperationRequest{Resource: ResourceRef{Provider: test.provider, Target: "local", Kind: KindContainer, ResourceID: test.id}, Operation: OperationExec, Command: []string{"cat"}}, machine.OperationContext{Environment: test.environment}, 24, 80)
		if err == nil || backend.opens != 0 {
			t.Fatal("terminal escaped ownership/environment/provider admission", test, err)
		}
	}
	_, err = service.Terminal(context.Background(), OperationRequest{Resource: ResourceRef{Provider: "docker", Target: "local", Kind: KindContainer, ResourceID: "owned"}, Operation: OperationExec, Command: []string{"cat"}}, machine.OperationContext{Environment: "dev"}, 24, 80)
	if err == nil || backend.opens != 1 {
		t.Fatal("owned resource did not reach typed backend")
	}
	classified := machine.Classify(err)
	if classified == nil {
		t.Fatal("runtime error cannot be classified")
	}
}
