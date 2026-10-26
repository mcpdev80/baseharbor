package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ContainerVolume records mounted volume identities before containers disappear.
// Anonymous ownership comes from the engine, never a name or creation time.
type ContainerVolume struct {
	Name      string
	Removable bool
	Reason    string
}

type volumeConsumer struct{ Name, Project string }

func (c Compose) InventoryOwnedContainerVolumes(ctx context.Context, project string) ([]ContainerVolume, error) {
	listed, err := c.directOutput(ctx, "container", "ls", "-aq")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(listed)
	if len(ids) == 0 {
		return nil, nil
	}
	args := append([]string{"container", "inspect", "--format", `{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "io.podman.compose.project" }}|{{json .Mounts}}`}, ids...)
	inspected, err := c.directOutput(ctx, args...)
	if err != nil {
		return nil, err
	}
	consumers := map[string][]volumeConsumer{}
	for _, line := range strings.Split(strings.TrimSpace(inspected), "\n") {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid container mount inventory")
		}
		var mounts []struct{ Type, Name string }
		if err := json.Unmarshal([]byte(parts[3]), &mounts); err != nil {
			return nil, fmt.Errorf("decode container mounts: %w", err)
		}
		for _, mount := range mounts {
			if mount.Type != "volume" {
				continue
			}
			if mount.Name == "" {
				return nil, fmt.Errorf("volume mount has no engine identity")
			}
			consumers[mount.Name] = append(consumers[mount.Name], volumeConsumer{strings.TrimPrefix(parts[0], "/"), firstRuntimeLabel(parts[1], parts[2])})
		}
	}
	var names []string
	for name, owners := range consumers {
		for _, owner := range owners {
			if owner.Project == project {
				names = append(names, name)
				break
			}
		}
	}
	sort.Strings(names)
	var volumes []ContainerVolume
	for _, name := range names {
		out, err := c.directOutput(ctx, "volume", "inspect", "--format", "{{json .}}", name)
		if err != nil {
			return nil, fmt.Errorf("inspect mounted volume %s: %w", name, err)
		}
		var metadata struct {
			Name      string
			Labels    map[string]string
			Anonymous *bool
		}
		if err := json.Unmarshal([]byte(out), &metadata); err != nil || metadata.Name != name {
			return nil, fmt.Errorf("invalid metadata for mounted volume %s", name)
		}
		_, dockerAnonymous := metadata.Labels["com.docker.volume.anonymous"]
		anonymous := dockerAnonymous || (metadata.Anonymous != nil && *metadata.Anonymous)
		volume := ContainerVolume{Name: name, Removable: anonymous, Reason: "engine-proven anonymous volume; all consumers belong to destroyed project"}
		if !anonymous {
			volume.Reason = "named or external volume; container removal preserves it"
		}
		for _, owner := range consumers[name] {
			if owner.Project != project {
				volume.Removable = false
				volume.Reason = "volume is shared with a container outside the destroyed project"
			}
		}
		volumes = append(volumes, volume)
	}
	return volumes, nil
}

func (c Compose) ContainerVolumeExists(ctx context.Context, name string) (bool, error) {
	out, err := c.directOutput(ctx, "volume", "ls", "--format", "{{.Name}}")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}
	return false, nil
}
