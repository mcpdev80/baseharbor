package providerbinding

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/providerupgrade"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// RuntimeReader is the read-only subset of the existing RuntimeProvider.
// No Docker/Podman command shell or independent control plane is introduced.
type RuntimeReader interface {
	ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error)
	ProjectServiceImageIdentity(context.Context, string, string) (bhruntime.ImageIdentity, error)
}

// ManagedSource names only existing, installation-owned Core services.
// Names are configuration/evidence selectors, never user-supplied mutation targets.
type ManagedSource struct {
	Project string
	Service string
}

// RuntimeBinding verifies ownership from live project/service labels and
// resolves the actual image/digest through the selected BaseHarbor RuntimeProvider.
type RuntimeBinding struct {
	Reader  RuntimeReader
	Engine  string
	Sources map[providerupgrade.Provider]ManagedSource
}

var _ RuntimeInventory = (*RuntimeBinding)(nil)

func (r *RuntimeBinding) InspectManaged(ctx context.Context, p providerupgrade.Provider) (RuntimeIdentity, error) {
	if r == nil || r.Reader == nil {
		return RuntimeIdentity{}, errors.New("selected Core RuntimeProvider is unavailable")
	}
	source, ok := r.Sources[p]
	if !ok || strings.TrimSpace(source.Project) == "" || strings.TrimSpace(source.Service) == "" {
		return RuntimeIdentity{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "provider discovery", errors.New("no installation-owned provider source configured"))
	}
	containers, err := r.Reader.ListRuntimeContainers(ctx)
	if err != nil {
		return RuntimeIdentity{}, fmt.Errorf("read RuntimeProvider inventory: %w", err)
	}
	count := 0
	for _, container := range containers {
		if container.Service == source.Service && container.Project != source.Project {
			// A service name collision is not evidence of ownership.
			continue
		}
		if container.Project != source.Project || container.Service != source.Service {
			continue
		}
		count++
		if !container.Running {
			return RuntimeIdentity{}, errors.New("managed provider has stopped or unhealthy members")
		}
		if strings.EqualFold(container.Health, "unhealthy") {
			return RuntimeIdentity{}, errors.New("managed provider has unhealthy members")
		}
	}
	if count == 0 {
		return RuntimeIdentity{}, providerupgrade.Wrap(providerupgrade.ErrorUnsupportedPath, "runtime inventory", errors.New("owned provider runtime not found"))
	}
	image, err := r.Reader.ProjectServiceImageIdentity(ctx, source.Project, source.Service)
	if err != nil {
		return RuntimeIdentity{}, fmt.Errorf("read verified provider image identity: %w", err)
	}
	digest := strings.TrimSpace(image.Digest)
	if at := strings.IndexByte(digest, '@'); at >= 0 {
		// Docker reports RepoDigests as repository@sha256; native Podman can
		// report the bare digest. Both must identify the observed repository.
		if imageRepository(digest[:at]) != imageRepository(image.Reference) {
			return RuntimeIdentity{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "runtime image", errors.New("immutable image repository differs"))
		}
		digest = digest[at+1:]
	}
	_, digestErr := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	if image.Reference == "" || !strings.HasPrefix(digest, "sha256:") || len(digest) != 71 || digestErr != nil {
		return RuntimeIdentity{}, providerupgrade.Wrap(providerupgrade.ErrorInvalidState, "runtime image", errors.New("immutable image digest unavailable"))
	}
	return RuntimeIdentity{Provider: p, Project: source.Project, Service: source.Service, Engine: r.Engine, Image: image.Reference, Digest: digest, Owned: true}, nil
}

func imageRepository(reference string) string {
	reference = strings.SplitN(strings.TrimSpace(reference), "@", 2)[0]
	if colon := strings.LastIndexByte(reference, ':'); colon > strings.LastIndexByte(reference, '/') {
		reference = reference[:colon]
	}
	for _, prefix := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		reference = strings.TrimPrefix(reference, prefix)
	}
	return reference
}
