package contract

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
)

type seamTestProvider struct{}

func (seamTestProvider) Kind() ProviderKind { return ProviderKubernetes }
func (seamTestProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{WorkloadLifecycle: true}
}
func (seamTestProvider) TargetScope() string { return "test" }
func (seamTestProvider) Apply(context.Context, runtimemodel.WorkloadPlan) error { return nil }
func (seamTestProvider) WaitReady(context.Context, runtimemodel.WorkloadPlan, time.Duration) error {
	return nil
}
func (seamTestProvider) Observe(context.Context, string, string, string) (runtimemodel.Observation, error) {
	return runtimemodel.Observation{}, nil
}
func (seamTestProvider) Logs(context.Context, string, string, string, string, int) (string, error) {
	return "", nil
}
func (seamTestProvider) Exec(context.Context, string, string, string, string, ...string) (string, error) {
	return "", nil
}
func (seamTestProvider) Destroy(context.Context, string, string, string) error { return nil }

var _ InternalWorkloadProvider = seamTestProvider{}

func TestInternalWorkloadProviderRemainsRuntimeOnly(t *testing.T) {
	typ := reflect.TypeOf((*InternalWorkloadProvider)(nil)).Elem()
	got := make([]string, 0, typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		got = append(got, typ.Method(i).Name)
	}
	sort.Strings(got)

	want := []string{
		"Apply",
		"Capabilities",
		"Destroy",
		"Exec",
		"Kind",
		"Logs",
		"Observe",
		"TargetScope",
		"WaitReady",
	}
	sort.Strings(want)

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("InternalWorkloadProvider methods = %v, want %v", got, want)
	}

	forbidden := map[string]bool{
		"Provision": true,
		"Bind": true,
		"Unbind": true,
		"Backup": true,
		"Restore": true,
		"Reconcile": true,
		"Deliver": true,
	}
	for _, method := range got {
		if forbidden[method] {
			t.Fatalf("internal runtime seam contains capability/delivery method %q", method)
		}
	}
}
