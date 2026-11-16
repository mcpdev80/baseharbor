package runtimeexplorer

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

type testMutationAuthority func(context.Context, Resource, OperationRequest) (MutationVerifier, error)

func (f testMutationAuthority) AuthorizeRuntimeMutation(ctx context.Context, r Resource, q OperationRequest) (MutationVerifier, error) {
	return f(ctx, r, q)
}

func sourceTestMutationAuthority() MutationAuthority {
	return testMutationAuthority(func(context.Context, Resource, OperationRequest) (MutationVerifier, error) {
		return func(context.Context, Resource, error) (*application.ProviderImpactScopeResult, error) {
			return &application.ProviderImpactScopeResult{Status: "partial"}, nil
		}, nil
	})
}

func TestRuntimeMutationCannotBypassRequiredAuthority(t *testing.T) {
	backend := &fakeBackend{containers: []runtimecontract.RuntimeContainer{{ID: "owned", Project: "bh-demo", Service: "api", Running: true, State: "running"}}}
	service, err := NewService(backend, "local", fakeResolver{})
	if err != nil {
		t.Fatal(err)
	}
	request := OperationRequest{Resource: ResourceRef{Provider: "docker", Target: "local", Kind: KindContainer, ResourceID: "owned"}, Operation: OperationStop}
	if _, err := service.Operate(t.Context(), request); err == nil || len(backend.operated) != 0 {
		t.Fatal("native mutation bypassed consequence authority")
	}
	service.authority = testMutationAuthority(func(context.Context, Resource, OperationRequest) (MutationVerifier, error) {
		return nil, errors.New("second acknowledgement required")
	})
	if _, err := service.Operate(t.Context(), request); err == nil || len(backend.operated) != 0 {
		t.Fatal("native mutation bypassed denied acknowledgement")
	}
	service.authority = sourceTestMutationAuthority()
	result, err := service.Operate(t.Context(), request)
	if err != nil || len(backend.operated) != 1 || result.ProviderChangeResult == nil || result.ProviderChangeResult.Status != "partial" {
		t.Fatalf("acknowledged bounded operation failed or fabricated full consequences: %+v %v", result, err)
	}
}

type partialMutationBackend struct{ fakeBackend }

func (b *partialMutationBackend) OperateContainer(ctx context.Context, id string, op Operation, args []string) (string, error) {
	b.fakeBackend.OperateContainer(ctx, id, op, args)
	return "", errors.New("command failed after stop")
}

func TestRuntimeMutationVerifiesPartialBackendFailure(t *testing.T) {
	backend := &partialMutationBackend{fakeBackend{containers: []runtimecontract.RuntimeContainer{{ID: "owned", Project: "bh-demo", Service: "api", Running: true, State: "running"}}}}
	called := false
	authority := testMutationAuthority(func(context.Context, Resource, OperationRequest) (MutationVerifier, error) {
		return func(_ context.Context, actual Resource, err error) (*application.ProviderImpactScopeResult, error) {
			called = true
			if actual.State.Observed != "exited" || err == nil {
				t.Fatal("verifier lost actual partial mutation")
			}
			return &application.ProviderImpactScopeResult{Status: "partial"}, nil
		}, nil
	})
	service, err := NewService(backend, "local", fakeResolver{}, authority)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Operate(t.Context(), OperationRequest{Resource: ResourceRef{Provider: "docker", Target: "local", Kind: KindContainer, ResourceID: "owned"}, Operation: OperationStop})
	if err == nil || !called || result.State.Observed != "exited" || result.ProviderChangeResult == nil {
		t.Fatal("failure discarded actual state or receipt")
	}
}

func TestRuntimeMutationRevalidatesNativeStateAfterAcknowledgement(t *testing.T) {
	backend := &fakeBackend{containers: []runtimecontract.RuntimeContainer{{ID: "owned", Project: "bh-demo", Service: "api", Running: true, State: "running"}}}
	authority := testMutationAuthority(func(ctx context.Context, r Resource, q OperationRequest) (MutationVerifier, error) {
		backend.containers[0].State = "changed"
		return sourceTestMutationAuthority().AuthorizeRuntimeMutation(ctx, r, q)
	})
	service, err := NewService(backend, "local", fakeResolver{}, authority)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Operate(t.Context(), OperationRequest{Resource: ResourceRef{Provider: "docker", Target: "local", Kind: KindContainer, ResourceID: "owned"}, Operation: OperationStop}); err == nil || len(backend.operated) != 0 {
		t.Fatal("stale native observation authorized mutation")
	}
}
