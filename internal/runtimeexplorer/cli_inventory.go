package runtimeexplorer

import (
	"context"
	"fmt"
	"strings"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

func (b *CLIContainerBackend) InventoryResourceKinds() []ResourceKind {
	kinds := []ResourceKind{KindImage, KindVolume, KindNetwork}
	if b.runtime.Kind() == runtimecontract.ProviderPodman {
		kinds = append(kinds, KindPod)
	}
	return kinds
}

func (b *CLIContainerBackend) ListInventoryResources(ctx context.Context, kind ResourceKind) ([]InventoryResource, error) {
	switch kind {
	case KindImage:
		return b.listImages(ctx)
	case KindVolume:
		return b.listVolumes(ctx)
	case KindNetwork:
		return b.listNetworks(ctx)
	case KindPod:
		if b.runtime.Kind() != runtimecontract.ProviderPodman {
			return nil, fmt.Errorf("runtime resource kind %q is not supported by provider %q", kind, b.runtime.Kind())
		}
		return b.listPods(ctx)
	default:
		return nil, fmt.Errorf("runtime resource kind %q is not supported by inventory backend", kind)
	}
}

func (b *CLIContainerBackend) listImages(ctx context.Context) ([]InventoryResource, error) {
	out, err := b.runtime.DirectOutput(ctx, "image", "ls", "--no-trunc", "--format", "{{.ID}}|{{.Repository}}|{{.Tag}}")
	if err != nil {
		return nil, err
	}
	var result []InventoryResource
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		repository := strings.TrimSpace(parts[1])
		tag := strings.TrimSpace(parts[2])
		name := repository
		if tag != "" && tag != "<none>" {
			name += ":" + tag
		}
		result = append(result, InventoryResource{
			Kind:       KindImage,
			ResourceID: strings.TrimSpace(parts[0]),
			Name:       name,
			References: map[string]string{"runtime.repository": repository, "runtime.tag": tag},
		})
	}
	return result, nil
}

func (b *CLIContainerBackend) listVolumes(ctx context.Context) ([]InventoryResource, error) {
	out, err := b.runtime.DirectOutput(ctx, "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil, err
	}
	var result []InventoryResource
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		result = append(result, InventoryResource{Kind: KindVolume, ResourceID: name, Name: name})
	}
	return result, nil
}

func (b *CLIContainerBackend) listNetworks(ctx context.Context) ([]InventoryResource, error) {
	listed, err := b.runtime.DirectOutput(ctx, "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(listed, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	args := []string{"network", "inspect", "--format", "{{.Id}}|{{.Name}}"}
	args = append(args, names...)
	inspected, err := b.runtime.DirectOutput(ctx, args...)
	if err != nil {
		return nil, err
	}
	var result []InventoryResource
	for _, line := range strings.Split(inspected, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		result = append(result, InventoryResource{
			Kind:       KindNetwork,
			ResourceID: strings.TrimSpace(parts[0]),
			Name:       strings.TrimSpace(parts[1]),
		})
	}
	return result, nil
}

func (b *CLIContainerBackend) listPods(ctx context.Context) ([]InventoryResource, error) {
	out, err := b.runtime.DirectOutput(ctx, "pod", "ps", "-a", "--no-trunc", "--format", "{{.ID}}|{{.Name}}|{{.Status}}")
	if err != nil {
		return nil, err
	}
	var result []InventoryResource
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" {
			continue
		}
		result = append(result, InventoryResource{
			Kind:       KindPod,
			ResourceID: strings.TrimSpace(parts[0]),
			Name:       strings.TrimSpace(parts[1]),
			State:      strings.TrimSpace(parts[2]),
		})
	}
	return result, nil
}

var _ InventoryBackend = (*CLIContainerBackend)(nil)
