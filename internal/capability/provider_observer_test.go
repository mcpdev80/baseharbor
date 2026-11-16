package capability

import (
	"context"
	"errors"
	"testing"
)

type recordingProviderObserver struct {
	observations []ProviderOperationObservation
}

func (o *recordingProviderObserver) ObserveProviderOperation(observation ProviderOperationObservation) {
	o.observations = append(o.observations, observation)
}

type observerDriver struct {
	provider Provider
	failAt   Phase
}

func (d observerDriver) Descriptor() Provider { return d.provider }
func (d observerDriver) Preflight(context.Context, Resource, Binding) error {
	if d.failAt == PhasePreflight {
		return errors.New("preflight failed")
	}
	return nil
}
func (d observerDriver) Provision(context.Context, Resource, Binding) error {
	if d.failAt == PhaseApply {
		return errors.New("apply failed")
	}
	return nil
}
func (d observerDriver) Bind(context.Context, Resource, Binding) error {
	if d.failAt == PhaseBind {
		return errors.New("bind failed")
	}
	return nil
}
func (d observerDriver) Verify(context.Context, Resource, Binding) error {
	if d.failAt == PhaseVerify {
		return errors.New("verify failed")
	}
	return nil
}

func TestProviderLifecycleObserverReceivesMetadataOnly(t *testing.T) {
	observer := &recordingProviderObserver{}
	request := Request{
		Requirement: Requirement{Kind: SQL, Name: "primary"},
		Workload:    "application/demo",
		Driver:      observerDriver{provider: PostgreSQL},
		Observer:    observer,
	}
	if _, err := Run(context.Background(), "demo", []Request{request}); err != nil {
		t.Fatal(err)
	}
	if len(observer.observations) != 4 {
		t.Fatalf("observations = %d, want 4", len(observer.observations))
	}
	want := []Phase{PhasePreflight, PhaseApply, PhaseBind, PhaseVerify}
	for i, observation := range observer.observations {
		if observation.Phase != want[i] || observation.Status != StatusReady {
			t.Fatalf("observation[%d] = %#v", i, observation)
		}
		if observation.Application != "demo" || observation.Capability != SQL || observation.Resource != "primary" || observation.Provider != ProviderPostgreSQL {
			t.Fatalf("observation[%d] identity = %#v", i, observation)
		}
		if observation.Duration < 0 {
			t.Fatalf("observation[%d] duration = %v", i, observation.Duration)
		}
	}
}

func TestProviderLifecycleObserverReportsFailureWithoutBindingPayload(t *testing.T) {
	observer := &recordingProviderObserver{}
	request := Request{
		Requirement: Requirement{Kind: SQL, Name: "primary"},
		Workload:    "application/demo",
		Security:    &SecureBinding{Credentials: []CredentialReference{{Name: "password", Reference: "baseharbor://applications/demo/prod/sql/primary/credentials/password"}}},
		Driver:      observerDriver{provider: PostgreSQL, failAt: PhaseBind},
		Observer:    observer,
	}
	_, err := Run(context.Background(), "demo", []Request{request})
	if err == nil {
		t.Fatal("expected bind failure")
	}
	last := observer.observations[len(observer.observations)-1]
	if last.Phase != PhaseBind || last.Status != StatusFailed {
		t.Fatalf("last observation = %#v", last)
	}
}
