package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

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
			if enriched, err := c.enrichServiceStatesFromRuntimeContainers(ctx, project, states); err == nil {
				states = enriched
			}
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

func (c Compose) ServiceStatesFromRuntimeLabels(ctx context.Context, project string) ([]ServiceState, error) {
	return c.serviceStatesFromRuntimeLabels(ctx, project)
}

func (c Compose) serviceStatesFromRuntimeLabels(ctx context.Context, project string) ([]ServiceState, error) {
	containers, err := c.ListRuntimeContainers(ctx)
	if err != nil {
		return nil, err
	}

	byService := map[string]ServiceState{}
	for _, container := range containers {
		if container.Project != project {
			continue
		}
		state := "exited"
		if container.Running {
			state = "running"
		}
		current, exists := byService[container.Service]
		if !exists || (current.State != "running" && state == "running") {
			if strings.TrimSpace(container.State) != "" {
				state = container.State
			}
			byService[container.Service] = ServiceState{
				Service:  container.Service,
				State:    state,
				Health:   container.Health,
				ExitCode: container.ExitCode,
				Error:    container.Error,
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
	ExitCode   int                  `json:"ExitCode"`
	Error      string               `json:"Error"`
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
			ExitCode:   row.ExitCode,
			Error:      strings.TrimSpace(row.Error),
			Publishers: publishers,
		})
	}
	return states, nil
}


func (c Compose) enrichServiceStatesFromRuntimeContainers(ctx context.Context, project string, states []ServiceState) ([]ServiceState, error) {
	containers, err := c.ListRuntimeContainers(ctx)
	if err != nil {
		return nil, err
	}
	byService := map[string]RuntimeContainer{}
	for _, container := range containers {
		if container.Project != project {
			continue
		}
		current, exists := byService[container.Service]
		if !exists || (!current.Running && container.Running) {
			byService[container.Service] = container
		}
	}
	result := append([]ServiceState(nil), states...)
	for i := range result {
		container, ok := byService[result[i].Service]
		if !ok {
			continue
		}
		if strings.TrimSpace(container.State) != "" {
			result[i].State = container.State
		}
		if strings.TrimSpace(result[i].Health) == "" {
			result[i].Health = container.Health
		}
		result[i].ExitCode = container.ExitCode
		result[i].Error = container.Error
	}
	return result, nil
}
