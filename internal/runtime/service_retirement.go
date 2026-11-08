package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// RuntimeServiceRetirementProvider retires a selected service set without
// bringing down sibling modules consolidated under the same Compose project.
type RuntimeServiceRetirementProvider interface {
	InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error)
	ListRuntimeContainers(context.Context) ([]RuntimeContainer, error)
	RetireOwnedProjectContainers(context.Context, string, []ProjectResource) error
}

// renderConfigProvider is the minimal optional render capability the retirement
// identity check needs. A provider that cannot render the model keeps the
// fail-closed canonical-only fallback (0427-128 r5: an unverifiable render path
// must never run as an empty planned-name set).
type renderConfigProvider interface {
	ConfigJSONProjectFilesEnv(context.Context, string, string, map[string]string, ...string) (string, error)
}

// nilCheckedRenderer reports whether the optional render seam is actually
// callable. It refuses an untyped nil and a bare nil *Compose receiver. The
// production wrappers (docker/podman Provider) embed Compose BY VALUE and keep
// their render-capable state in exported pointer fields, so the walk refuses an
// exported nil pointer whose method set could be promoted (e.g. an embedded
// *RuntimeProvider that was never initialised): calling through those
// dereferences nil and panics (0427-128 r5: the hosted retirement panics).
// Unexported fields are skipped: encoding a private-state policy in a shared
// identity path would let a zero-value Compose (still callable, failing only
// with a render error into the canonical fallback) be misread as uncallable.
func nilCheckedRenderer(configurer renderConfigProvider) bool {
	if configurer == nil {
		return false
	}
	return valueRendererReady(reflect.ValueOf(configurer))
}

func valueRendererReady(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Ptr:
		if value.IsNil() {
			return false
		}
		target := value.Elem()
		if target.Kind() != reflect.Struct {
			return true
		}
		for field := range target.NumField() {
			if !target.Type().Field(field).IsExported() {
				continue
			}
			if !valueRendererReady(target.Field(field)) {
				return false
			}
		}
		return true
	case reflect.Struct:
		for field := range value.NumField() {
			if !value.Type().Field(field).IsExported() {
				continue
			}
			if !valueRendererReady(value.Field(field)) {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// plannedNamesForService renders the exact planned native container names for
// one selected service. A provider without the render capability, a render
// failure, an empty model and a malformed model all yield no set so the caller
// falls back to the canonical ordinal name fail-closed. The optional call is
// panic-safe: a provider that only pretends to render (e.g. an embedded nil
// promoted through a wrapper) is detected and the canonical fallback applies
// (0427-128 r5: the hosted retirement panics).
func plannedNamesForService(ctx context.Context, provider any, project, service string) (names []string) {
	configurer, ok := provider.(renderConfigProvider)
	if !ok || !nilCheckedRenderer(configurer) {
		return nil
	}
	defer func() {
		// Last resort for renderers whose nil path the seam cannot inspect (a
		// panic inside the implementation): the caller keeps the canonical
		// fail-closed fallback instead of crashing the retirement.
		if recover() != nil {
			names = nil
		}
	}()
	rendered, err := configurer.ConfigJSONProjectFilesEnv(ctx, project, "", nil)
	if err != nil || rendered == "" {
		return nil
	}
	parsed, perr := parsedExpectedContainerNames(rendered, project, service)
	if perr != nil {
		return nil
	}
	return parsed
}

// serviceFromCanonical derives the service a canonical inventory resource name
// targets. The canonical owner name is <project>-<service>-1; a pinned
// container_name (0427-128) lives outside that scheme as <project>-<service>,
// so the remainder after the project prefix is the service identifier in both
// cases. A name without the owning project prefix is not an owner name.
func serviceFromCanonical(resourceName, project string) string {
	if !strings.HasPrefix(resourceName, project+"-") {
		return ""
	}
	service := strings.TrimSuffix(strings.TrimPrefix(resourceName, project+"-"), "-1")
	if service == "" {
		return ""
	}
	return service
}

// RetireOwnedServices accepts the canonical old resource set and exactly the
// containers released by the consumer plan. Volumes and networks are never
// deletion targets. Failed mutations still return observed partial disposition.
func RetireOwnedServices(ctx context.Context, provider RuntimeServiceRetirementProvider, project string, expected, remove []ProjectResource) (RuntimeRetirement, error) {
	result := RuntimeRetirement{Removed: []ProjectResource{}, Retained: []RetainedRuntimeResource{}}
	if provider == nil || strings.TrimSpace(project) == "" || len(expected) == 0 {
		return result, errors.New("service retirement requires canonical owner inventory")
	}
	planned := map[ProjectResource]bool{}
	for _, resource := range expected {
		if strings.TrimSpace(resource.Name) == "" || planned[resource] || (resource.Kind != "container" && resource.Kind != "network" && resource.Kind != "volume") {
			return result, errors.New("service retirement inventory is invalid or duplicated")
		}
		planned[resource] = true
	}
	services := map[string]bool{}
	removed := map[ProjectResource]bool{}
	for _, resource := range remove {
		if resource.Kind != "container" || !planned[resource] || removed[resource] || !strings.HasPrefix(resource.Name, project+"-") || strings.TrimSpace(resource.Name) == project+"-" {
			return result, errors.New("service retirement requires an owned canonical container name")
		}
		removed[resource] = true
	}
	before, err := provider.InspectProjectResources(ctx, project, expected)
	if err != nil {
		return result, fmt.Errorf("verify selected service ownership: %w", err)
	}
	for _, resource := range before {
		if !planned[resource] {
			return result, errors.New("service retirement observation contains an unplanned resource")
		}
	}
	containers, err := provider.ListRuntimeContainers(ctx)
	if err != nil {
		return result, err
	}
	kind := ProviderDocker
	if native, ok := provider.(interface{ Kind() ProviderKind }); ok {
		kind = native.Kind()
	}
	// Pinned container_name lives outside <project>-<service>-1, so the retired
	// set also carries the rendered planned native names per selected service.
	// The canonical name still owns the service; a matching native name alone
	// never infers ownership (0427-128: retirement accepts both identity
	// classes). The rendered model is the source of truth for the planned
	// names; an unverifiable render keeps the canonical-only fallback.
	plannedByService := map[string][]string{}
	for _, resource := range expected {
		if resource.Kind != "container" {
			continue
		}
		if service := serviceFromCanonical(resource.Name, project); service != "" {
			services[service] = true
		}
	}
	for service := range services {
		if expectedNames := plannedNamesForService(ctx, any(provider), project, service); len(expectedNames) > 0 {
			plannedByService[service] = expectedNames
		}
	}
	for _, container := range containers {
		if container.Project == project && services[container.Service] {
			plannedNames := plannedByService[container.Service]
			if len(plannedNames) == 0 {
				// No rendered model: fall back to the single canonical ordinal so
				// the pre-existing canonical-owner contract is preserved.
				plannedNames = []string{project + "-" + container.Service + "-1"}
			}
			if !ContainerNameInPlannedSet(kind, container, project, plannedNames) {
				return result, fmt.Errorf("unplanned replica of selected service %s; preserve ownership state", container.Service)
			}
		}
	}
	var mutationErr error
	if len(remove) > 0 {
		// Pass only the explicitly selected containers. The existing native
		// adapter rechecks their ownership and never receives a data volume.
		mutationErr = provider.RetireOwnedProjectContainers(ctx, project, remove)
	}
	after, err := provider.InspectProjectResources(ctx, project, expected)
	if err != nil {
		return result, errors.Join(mutationErr, fmt.Errorf("verify selected retirement; preserve ownership for retry: %w", err))
	}
	remaining := map[ProjectResource]bool{}
	var verificationErr error
	for _, resource := range after {
		if !planned[resource] {
			return result, errors.Join(mutationErr, errors.New("service retirement verification contains an unplanned resource"))
		}
		remaining[resource] = true
		if removed[resource] {
			verificationErr = errors.Join(verificationErr, fmt.Errorf("retired container remains: %s", resource.Name))
		} else {
			reason := "resource remains with its existing owner for active consumers and retry"
			if resource.Kind == "network" {
				reason = "owned network remains as existing module/retry state; selected container retirement does not verify attachment release or authorize network reclamation"
			}
			if resource.Kind == "volume" {
				reason = "persistent volume remains at its existing location; data deletion requires separate explicit consent and ownership proof"
			}
			result.Retained = append(result.Retained, RetainedRuntimeResource{Resource: resource, Reason: reason})
		}
	}
	for _, resource := range before {
		if remaining[resource] {
			continue
		}
		if !removed[resource] {
			verificationErr = errors.Join(verificationErr, fmt.Errorf("selected retirement lost non-selected %s %s; preserve recovery state", resource.Kind, resource.Name))
		}
		result.Removed = append(result.Removed, resource)
	}
	return result, errors.Join(mutationErr, verificationErr)
}
