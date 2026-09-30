package artifact

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Builder converts repository source into an OCI artifact at a deployment-owned
// distribution reference. OCI identity/distribution is the contract; the build
// engine remains an implementation detail.
type Builder interface {
	Build(context.Context, BuildRequest, string) (Artifact, error)
}

// BuildDestinations maps workload service -> deployment-owned target image
// reference (for example a registry repository/tag).
type BuildDestinations map[string]string

// CompleteResolution builds every unresolved source request and returns one
// complete artifact set. Existing image-backed artifacts pass through.
func CompleteResolution(
	ctx context.Context,
	resolution Resolution,
	destinations BuildDestinations,
	builder Builder,
) (Resolution, error) {
	if builder == nil && len(resolution.Builds) > 0 {
		return Resolution{}, fmt.Errorf("artifact builder is required for source-backed workload services")
	}

	result := Resolution{
		Artifacts: append([]Artifact(nil), resolution.Artifacts...),
	}
	builds := append([]BuildRequest(nil), resolution.Builds...)
	sort.Slice(builds, func(i, j int) bool { return builds[i].Service < builds[j].Service })

	for _, request := range builds {
		destination := strings.TrimSpace(destinations[request.Service])
		if destination == "" {
			return Resolution{}, fmt.Errorf(
				"source-backed workload service %q has no deployment artifact destination",
				request.Service,
			)
		}
		built, err := builder.Build(ctx, request, destination)
		if err != nil {
			return Resolution{}, fmt.Errorf("build artifact for workload service %s: %w", request.Service, err)
		}
		if built.Service == "" {
			built.Service = request.Service
		}
		if built.Service != request.Service {
			return Resolution{}, fmt.Errorf(
				"artifact builder returned service %q for requested service %q",
				built.Service,
				request.Service,
			)
		}
		if strings.TrimSpace(built.Reference) == "" {
			return Resolution{}, fmt.Errorf("artifact builder returned no OCI reference for service %q", request.Service)
		}
		result.Artifacts = append(result.Artifacts, built)
	}
	return result, nil
}
