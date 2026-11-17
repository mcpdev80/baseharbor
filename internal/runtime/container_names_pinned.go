package runtime

import "context"

// AppendPinnedContainerNames carries pinned container_name names into the
// validated stop inventory so resource inspection verifies the pinned resource.
// A pinned name lives outside <project>-<service>-1, so the Compose JSON render
// supplies it; the canonical <project>-<service>-1 name still selects and owns
// the service, and a matching native name alone never infers ownership
// (0427-128). Duplicate names are dropped to keep the stop inventory valid.
// The caller gates this on a concrete Compose provider: first-party Podman uses
// the project-service name without an ordinal and is not covered here.
func AppendPinnedContainerNames(ctx context.Context, provider RuntimeConfigProvider, project, workdir string, environment map[string]string, composeFiles []string, services []string, expected []ProjectResource) []ProjectResource {
	if provider == nil {
		return expected
	}
	rendered, err := provider.ConfigJSONProjectFilesEnv(ctx, project, workdir, environment, composeFiles...)
	if err != nil || rendered == "" {
		return expected
	}
	seen := map[string]bool{}
	for _, resource := range expected {
		seen[resource.Name] = true
	}
	for _, service := range services {
		planned, perr := parsedExpectedContainerNames(rendered, project, service)
		if perr != nil {
			continue
		}
		for _, name := range planned {
			if seen[name] {
				continue
			}
			seen[name] = true
			expected = append(expected, ProjectResource{Kind: "container", Name: name})
		}
	}
	return expected
}
