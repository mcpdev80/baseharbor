package runtime

import (
	"context"
	"encoding/json"
	"fmt"
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
	out, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "ps", "--format", "json")
	if err != nil {
		fallback, fallbackErr := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "ps", "--services", "--status", "running")
		if fallbackErr != nil {
			return nil, err
		}
		services := nonEmptyLines(fallback)
		states := make([]ServiceState, 0, len(services))
		for _, service := range services {
			states = append(states, ServiceState{Service: service, State: "running"})
		}
		return states, nil
	}
	states, err := parseComposeServiceStates(out)
	if err != nil {
		return nil, fmt.Errorf("parse compose service state: %w", err)
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
