package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RepositoryVolume is observation only: Compose provenance never grants data deletion.
type RepositoryVolume struct {
	Name    string
	Project string
	Owner   string
	Reason  string
	Shared  bool
}

// InventoryRepositoryVolumes includes mounted data and orphaned project-labelled
// volumes, so repeating destroy still reports data after containers disappear.
func (c Compose) InventoryRepositoryVolumes(ctx context.Context, project string, previouslyObserved ...string) ([]RepositoryVolume, error) {
	mounted, err := c.InventoryOwnedContainerVolumes(ctx, project)
	if err != nil {
		return nil, err
	}
	seen := map[string]ContainerVolume{}
	previous := map[string]bool{}
	for _, name := range previouslyObserved {
		previous[name] = true
	}
	for _, volume := range mounted {
		seen[volume.Name] = volume
	}
	listed, err := c.directOutput(ctx, "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return nil, err
	}
	result := []RepositoryVolume{}
	for _, name := range strings.Fields(listed) {
		out, err := c.directOutput(ctx, "volume", "inspect", "--format", "{{json .}}", name)
		if err != nil {
			return nil, fmt.Errorf("inspect repository volume %s: %w", name, err)
		}
		var metadata struct {
			Name   string
			Labels map[string]string
		}
		if err := json.Unmarshal([]byte(out), &metadata); err != nil || metadata.Name != name {
			return nil, fmt.Errorf("invalid repository volume metadata for %s", name)
		}
		docker, podman := metadata.Labels["com.docker.compose.project"], metadata.Labels["io.podman.compose.project"]
		mount, attached := seen[name]
		if docker != project && podman != project && !attached && !previous[name] {
			continue
		}
		owner := metadata.Labels["io.baseharbor.owner"]
		if owner == "" {
			owner = "not recorded"
		}
		volume := RepositoryVolume{Name: name, Project: firstRuntimeLabel(docker, podman), Owner: owner, Reason: "persistent repository data; destroy consent does not authorize data-volume deletion"}
		if attached && strings.Contains(mount.Reason, "shared") {
			volume.Shared = true
			volume.Reason = mount.Reason
		}
		if (docker != "" && docker != project) || (podman != "" && podman != project) {
			volume.Shared = true
			volume.Reason = "foreign or conflicting Compose ownership"
		}
		if volume.Project == "" {
			volume.Reason = "mounted volume without verified Compose ownership"
		}
		for key, value := range metadata.Labels {
			if strings.Contains(strings.ToLower(key), "recovery") || strings.Contains(strings.ToLower(key), "backup") || strings.Contains(strings.ToLower(value), "recovery") {
				volume.Shared = true
				volume.Reason = "recovery/backup data; preserved"
			}
		}
		result = append(result, volume)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
