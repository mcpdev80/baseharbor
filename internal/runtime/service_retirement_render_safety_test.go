package runtime

import (
	"context"
	"testing"
)

// nilRenderRetirementProvider pretends to hold the optional render capability:
// the ConfigJSONProjectFilesEnv method is promoted from an embedded *nilBackedProvider
// that was never initialised, so the call dereferences nil. 0427-128 r5:
// RetireOwnedServices must treat this as "no rendered model" and keep the
// fail-closed canonical fallback instead of panicking (hosted run 42455350913
// go-unit-tests nil-pointer panics in the retirement path).
type nilRenderRetirementProvider struct {
	*nilBackedProvider
	containers []RuntimeContainer
	selected   []ProjectResource
	retired    int
	calls      int
}

type nilBackedProvider struct{}

func (n *nilBackedProvider) InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error) {
	return nil, nil
}

func (n *nilBackedProvider) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	return "", nil
}

func (r *nilRenderRetirementProvider) Kind() ProviderKind { return ProviderDocker }
func (r *nilRenderRetirementProvider) InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error) {
	r.calls++
	if r.calls == 1 {
		return []ProjectResource{{Kind: "container", Name: "owned-sql-1"}, {Kind: "volume", Name: "sql-data"}}, nil
	}
	return []ProjectResource{{Kind: "volume", Name: "sql-data"}}, nil
}
func (r *nilRenderRetirementProvider) ListRuntimeContainers(context.Context) ([]RuntimeContainer, error) {
	return r.containers, nil
}
func (r *nilRenderRetirementProvider) RetireOwnedProjectContainers(_ context.Context, _ string, selected []ProjectResource) error {
	r.retired++
	r.selected = append([]ProjectResource{}, selected...)
	return nil
}

// panicRenderProvider implements the render method directly but always panics,
// like a broken backend. The retirement path must stay panic-free.
type panicRenderProvider struct {
	withoutRenderProvider
}

// panicRenderProvider implements the render method directly but always panics.
// The nil-safe seam cannot inspect such an implementation, so the retirement
// path keeps a narrow last-resort guard around the optional call: the panic
// becomes "no rendered model" and the canonical fail-closed fallback applies.
func (r *panicRenderProvider) ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error) {
	panic("simulated broken renderer")
}

// withoutRenderProvider implements no render capability at all.
type withoutRenderProvider struct {
	containers    []RuntimeContainer
	before, after []ProjectResource
	selected      []ProjectResource
	mutations     int
	inspectCalls  int
}

func (r *withoutRenderProvider) Kind() ProviderKind { return ProviderDocker }
func (r *withoutRenderProvider) InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error) {
	r.inspectCalls++
	if r.inspectCalls == 1 {
		return r.before, nil
	}
	return r.after, nil
}
func (r *withoutRenderProvider) ListRuntimeContainers(context.Context) ([]RuntimeContainer, error) {
	return r.containers, nil
}
func (r *withoutRenderProvider) RetireOwnedProjectContainers(_ context.Context, _ string, selected []ProjectResource) error {
	r.mutations++
	r.selected = append([]ProjectResource{}, selected...)
	return nil
}

func TestRetireOwnedServicesStaysPanicFreeWithoutCallableRender(t *testing.T) {
	container := ProjectResource{Kind: "container", Name: "owned-sql-1"}
	volume := ProjectResource{Kind: "volume", Name: "sql-data"}

	// (a) embedded nil renderer: promoted method, panics on call.
	r := &nilRenderRetirementProvider{
		containers: []RuntimeContainer{{Name: "owned-sql-1", Project: "owned", Service: "sql"}},
	}
	if _, err := RetireOwnedServices(context.Background(), r, "owned", []ProjectResource{container, volume}, []ProjectResource{container}); err != nil {
		t.Fatalf("canonical retirement with an unverifiable renderer: %v", err)
	}

	// (b) provider without any render capability: canonical identity keeps
	// working and an unplanned replica is still rejected fail-closed.
	plain := &withoutRenderProvider{
		containers: []RuntimeContainer{{Name: "owned-sql-2", Project: "owned", Service: "sql"}},
		before:     []ProjectResource{container, volume},
		after:      []ProjectResource{container, volume},
	}
	if _, err := RetireOwnedServices(context.Background(), plain, "owned", []ProjectResource{container, volume}, []ProjectResource{container}); err == nil || plain.mutations != 0 {
		t.Fatal("unplanned replica accepted without a render capability", err)
	}
	plain = &withoutRenderProvider{
		containers: []RuntimeContainer{{Name: "owned-sql-1", Project: "owned", Service: "sql"}},
		before:     []ProjectResource{container, volume},
		after:      []ProjectResource{volume},
	}
	if _, err := RetireOwnedServices(context.Background(), plain, "owned", []ProjectResource{container, volume}, []ProjectResource{container}); err != nil || plain.mutations != 1 {
		t.Fatal("canonical retirement without a render capability", err)
	}

	// (c) a renderer that itself panics must not fail the production path.
	broken := &panicRenderProvider{withoutRenderProvider: withoutRenderProvider{
		containers: []RuntimeContainer{{Name: "owned-sql-1", Project: "owned", Service: "sql"}},
		before:     []ProjectResource{container, volume},
		after:      []ProjectResource{volume},
	}}
	if _, err := RetireOwnedServices(context.Background(), broken, "owned", []ProjectResource{container, volume}, []ProjectResource{container}); err != nil || broken.mutations != 1 {
		t.Fatal("canonical retirement with a panicking renderer", err)
	}
}
