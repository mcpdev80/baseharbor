package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// PublishedPort is the provider-facing portion of a Compose publisher that is
// useful for local operational readiness. It deliberately contains no
// BaseHarbor-specific ingress model; it mirrors existing Compose runtime state.
type PublishedPort struct {
	URL           string
	TargetPort    int
	PublishedPort int
	Protocol      string
}

// ServiceState is the provider-facing runtime state BaseHarbor needs for
// truthful workload readiness. Health is empty when the service has no
// healthcheck or the Compose implementation cannot report one.
type ServiceState struct {
	Service    string
	State      string
	Health     string
	Publishers []PublishedPort
}

// Ready reports whether the service is running and, when a health status is
// available, has reached a healthy terminal state.
func (s ServiceState) Ready() bool {
	if !strings.EqualFold(strings.TrimSpace(s.State), "running") {
		return false
	}
	health := strings.ToLower(strings.TrimSpace(s.Health))
	return health == "" || health == "healthy"
}

// ServiceStatesProjectFilesEnv returns Compose service state without exposing
// generated container names to application code. Modern Compose implementations
// expose JSON state including container health and published ports. Older
// compatible implementations fall back to the portable running-service query;
// in that case Health and Publishers remain empty rather than inventing state.
func (c Compose) ServiceStatesProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) ([]ServiceState, error) {
	out, composeErr := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "ps", "--format", "json")
	if composeErr == nil {
		states, parseErr := parseComposeServiceStates(out)
		if parseErr == nil && len(states) > 0 {
			return states, nil
		}
	}

	states, fallbackErr := c.serviceStatesFromRuntimeLabels(ctx, project)
	if fallbackErr == nil {
		return states, nil
	}
	if composeErr != nil {
		return nil, fmt.Errorf("%v; runtime-label fallback: %w", composeErr, fallbackErr)
	}
	return nil, fmt.Errorf("compose service state unavailable; runtime-label fallback: %w", fallbackErr)
}

func (c Compose) serviceStatesFromRuntimeLabels(ctx context.Context, project string) ([]ServiceState, error) {
	containers, err := c.ListComposeContainers(ctx)
	if err != nil {
		return nil, err
	}

	byService := map[string]ServiceState{}
	for _, container := range containers {
		if container.Project != project {
			continue
		}
		out, err := c.directOutput(ctx, "container", "inspect", "--format", `{{.State.Running}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}`, container.Name)
		if err != nil {
			return nil, fmt.Errorf("inspect state for %s: %w", container.Name, err)
		}
		runningRaw, healthRaw, _ := strings.Cut(strings.TrimSpace(out), "|")
		state := "exited"
		if strings.EqualFold(strings.TrimSpace(runningRaw), "true") {
			state = "running"
		}
		health := strings.TrimSpace(healthRaw)
		current, exists := byService[container.Service]
		if !exists || (current.State != "running" && state == "running") {
			byService[container.Service] = ServiceState{
				Service: container.Service,
				State:   state,
				Health:  health,
			}
		}
	}

	names := make([]string, 0, len(byService))
	for service := range byService {
		names = append(names, service)
	}
	sort.Strings(names)
	states := make([]ServiceState, 0, len(names))
	for _, service := range names {
		states = append(states, byService[service])
	}
	return states, nil
}

type composePSPublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

type composePSState struct {
	Service    string               `json:"Service"`
	State      string               `json:"State"`
	Health     string               `json:"Health"`
	Publishers []composePSPublisher `json:"Publishers"`
}

func parseComposeServiceStates(out string) ([]ServiceState, error) {
	data := strings.TrimSpace(out)
	if data == "" {
		return nil, nil
	}

	var rows []composePSState
	if strings.HasPrefix(data, "[") {
		if err := json.Unmarshal([]byte(data), &rows); err != nil {
			return nil, err
		}
	} else {
		for _, line := range strings.Split(data, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var row composePSState
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	}

	states := make([]ServiceState, 0, len(rows))
	for _, row := range rows {
		service := strings.TrimSpace(row.Service)
		if service == "" {
			continue
		}
		publishers := make([]PublishedPort, 0, len(row.Publishers))
		for _, publisher := range row.Publishers {
			publishers = append(publishers, PublishedPort{
				URL:           strings.TrimSpace(publisher.URL),
				TargetPort:    publisher.TargetPort,
				PublishedPort: publisher.PublishedPort,
				Protocol:      strings.ToLower(strings.TrimSpace(publisher.Protocol)),
			})
		}
		states = append(states, ServiceState{
			Service:    service,
			State:      strings.TrimSpace(row.State),
			Health:     strings.TrimSpace(row.Health),
			Publishers: publishers,
		})
	}
	return states, nil
}
