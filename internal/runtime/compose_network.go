package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (c Compose) PullImage(ctx context.Context, image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return errors.New("image reference is required")
	}
	if _, err := c.directOutput(ctx, "image", "pull", image); err != nil {
		return fmt.Errorf("pull image %s: %w", image, err)
	}
	return nil
}

func (c Compose) ContainerHealthStatus(ctx context.Context, container string) (string, error) {
	out, err := c.directOutput(ctx, "container", "inspect", "--format", `{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}`, strings.TrimSpace(container))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c Compose) ContainerNetworks(ctx context.Context, container string) ([]string, error) {
	out, err := c.directOutput(ctx, "container", "inspect", "--format", `{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{"\n"}}{{end}}`, strings.TrimSpace(container))
	if err != nil {
		return nil, err
	}
	var result []string
	for _, line := range strings.Split(out, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			result = append(result, value)
		}
	}
	return result, nil
}

func (c Compose) NetworkProjectOwner(ctx context.Context, network string) (string, error) {
	out, err := c.directOutput(ctx, "network", "inspect", "--format", `{{ index .Labels "com.docker.compose.project" }}`, strings.TrimSpace(network))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (c Compose) ContainerExposedTCPPorts(ctx context.Context, container string) ([]int, error) {
	out, err := c.directOutput(ctx, "container", "inspect", "--format", `{{range $port, $_ := .Config.ExposedPorts}}{{$port}}{{"\n"}}{{end}}`, strings.TrimSpace(container))
	if err != nil {
		return nil, err
	}
	var ports []int
	seen := map[int]struct{}{}
	for _, line := range strings.Split(out, "\n") {
		value := strings.TrimSpace(line)
		if value == "" || !strings.HasSuffix(value, "/tcp") {
			continue
		}
		raw := strings.TrimSuffix(value, "/tcp")
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports, nil
}

func (c Compose) EnsureManagedNetwork(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("managed network name is required")
	}
	out, err := c.directOutput(ctx, "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != name {
			continue
		}
		label, err := c.directOutput(ctx, "network", "inspect", "--format", `{{ index .Labels "io.baseharbor.managed" }}`, name)
		if err != nil {
			return err
		}
		if strings.TrimSpace(label) != "connectivity" {
			return fmt.Errorf("runtime network %s already exists but is not BaseHarbor connectivity-owned", name)
		}
		return nil
	}
	_, err = c.directOutput(ctx, "network", "create", "--label", "io.baseharbor.managed=connectivity", name)
	return err
}

func (c Compose) ConnectManagedNetwork(ctx context.Context, network, container, alias string) error {
	args := []string{"network", "connect"}
	if alias = strings.TrimSpace(alias); alias != "" {
		args = append(args, "--alias", alias)
	}
	args = append(args, strings.TrimSpace(network), strings.TrimSpace(container))
	_, err := c.directOutput(ctx, args...)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "already exists") || strings.Contains(message, "already connected") {
			return nil
		}
	}
	return err
}

func (c Compose) DisconnectManagedNetwork(ctx context.Context, network, container string) error {
	_, err := c.directOutput(ctx, "network", "disconnect", strings.TrimSpace(network), strings.TrimSpace(container))
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "not connected") || strings.Contains(message, "is not connected") {
			return nil
		}
	}
	return err
}

func (c Compose) RemoveManagedNetwork(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("managed network name is required")
	}
	out, err := c.directOutput(ctx, "network", "ls", "--format", "{{.Name}}")
	if err != nil {
		return err
	}
	found := false
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	label, err := c.directOutput(ctx, "network", "inspect", "--format", `{{ index .Labels "io.baseharbor.managed" }}`, name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(label) != "connectivity" {
		return fmt.Errorf("refusing to remove runtime network %s because it is not BaseHarbor connectivity-owned", name)
	}
	_, err = c.directOutput(ctx, "network", "rm", name)
	return err
}
