package targetsession

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

var projectName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)
var containerIdentity = regexp.MustCompile(`^[0-9a-f]{12,64}$`)

// ProjectService is an observation, not a readiness decision. Backend probes
// still have to verify the application's actual TLS and service contract.
type ProjectService struct {
	ID         string
	Service    string
	Running    bool
	Health     string
	State      string
	ExitCode   *int
	StartedAt  time.Time
	FinishedAt time.Time
}

// ObserveProject resolves a Core-selected project through live inventory and
// independently verifies each container's native ownership labels. Container
// names never establish ownership, including on rootless Quadlet targets.
func (r *ProjectRuntime) ObserveProject(ctx context.Context, project string) ([]ProjectService, error) {
	if !projectName.MatchString(project) {
		return nil, errors.New("invalid Core project selection")
	}
	var inventory []struct {
		ID      string `json:"id"`
		Project string `json:"compose_project"`
		Service string `json:"compose_service"`
	}
	if err := r.invoke(ctx, "runtime.resource.list", struct{}{}, &inventory); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var services []ProjectService
	for _, item := range inventory {
		if item.Project != project {
			continue
		}
		if !containerIdentity.MatchString(item.ID) || !projectName.MatchString(item.Service) || seen[item.ID] {
			return nil, errors.New("ambiguous remote project inventory")
		}
		seen[item.ID] = true
		var inspected []struct {
			ID     string `json:"Id"`
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
			State struct {
				Running    *bool     `json:"Running"`
				Status     string    `json:"Status"`
				ExitCode   *int      `json:"ExitCode"`
				StartedAt  time.Time `json:"StartedAt"`
				FinishedAt time.Time `json:"FinishedAt"`
				Health     struct {
					Status string `json:"Status"`
				} `json:"Health"`
			} `json:"State"`
		}
		if err := r.invoke(ctx, "runtime.resource.inspect", map[string]string{"resource_id": item.ID}, &inspected); err != nil {
			return nil, err
		}
		if len(inspected) != 1 {
			return nil, errors.New("remote project inspection is ambiguous")
		}
		container := inspected[0]
		if len(container.ID) != 64 || !containerIdentity.MatchString(container.ID) || !strings.HasPrefix(container.ID, item.ID) ||
			container.Config.Labels["com.docker.compose.project"] != project ||
			container.Config.Labels["com.docker.compose.service"] != item.Service || container.State.Running == nil {
			return nil, errors.New("remote project ownership or state differs")
		}
		services = append(services, ProjectService{ID: container.ID, Service: item.Service,
			Running: *container.State.Running, Health: container.State.Health.Status,
			State: container.State.Status, ExitCode: container.State.ExitCode,
			StartedAt: container.State.StartedAt, FinishedAt: container.State.FinishedAt})
	}
	return services, nil
}

// ExecService executes a Core-approved backend probe in exactly one owned,
// running service. It re-resolves the immutable container ID for every call;
// ambiguous replicas, foreign labels and missing exit status fail closed.
func (r *ProjectRuntime) ExecService(ctx context.Context, project, service string, argv ...string) (string, error) {
	if !projectName.MatchString(service) || len(argv) == 0 || len(argv) > 64 || argv[0] == "" {
		return "", errors.New("invalid Core service command")
	}
	total := 0
	for _, arg := range argv {
		total += len(arg)
		if strings.ContainsRune(arg, '\x00') || len(arg) > 4096 || total > 64<<10 {
			return "", errors.New("invalid Core service command")
		}
	}
	services, err := r.ObserveProject(ctx, project)
	if err != nil {
		return "", err
	}
	var selected *ProjectService
	for i := range services {
		if services[i].Service == service {
			if selected != nil || !services[i].Running {
				return "", errors.New("remote service is ambiguous or stopped")
			}
			selected = &services[i]
		}
	}
	if selected == nil {
		return "", errors.New("remote service is absent")
	}
	var result struct {
		Stdout   string `json:"stdout"`
		ExitCode *int   `json:"exit_code"`
	}
	if err := r.invoke(ctx, "runtime.exec", map[string]any{"resource_id": selected.ID,
		"argv": append([]string(nil), argv...), "timeout_seconds": 60}, &result); err != nil {
		return "", err
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		return "", errors.New("remote service command failed")
	}
	return result.Stdout, nil
}

// VerifyCompletedService verifies a one-shot through owned native container
// evidence. Stopped, never-started and missing/removed containers are not proof
// of success. Quadlet units that remove their container need a separately
// qualified unit-completion observation; this method never infers it.
func (r *ProjectRuntime) VerifyCompletedService(ctx context.Context, project, service string) error {
	if !projectName.MatchString(service) {
		return errors.New("invalid Core completion service selection")
	}
	observed, err := r.ObserveProject(ctx, project)
	if err != nil {
		return err
	}
	var selected *ProjectService
	for i := range observed {
		if observed[i].Service != service {
			continue
		}
		if selected != nil {
			return errors.New("remote completion service is ambiguous")
		}
		selected = &observed[i]
	}
	if selected == nil || selected.Running || selected.State != "exited" ||
		selected.ExitCode == nil || *selected.ExitCode != 0 || selected.StartedAt.IsZero() ||
		selected.FinishedAt.IsZero() || selected.FinishedAt.Before(selected.StartedAt) ||
		selected.FinishedAt.After(time.Now().Add(5*time.Second)) {
		return errors.New("remote service successful completion is unverified")
	}
	return nil
}

// RemoveOwnedService re-resolves ownership immediately before removing one
// exact native ID. Provider stop time fits the bounded project operation rather
// than a short probe deadline. A lost response is never replayed.
func (r *ProjectRuntime) RemoveOwnedService(ctx context.Context, project, service string, force bool) error {
	if !projectName.MatchString(service) {
		return errors.New("invalid Core cleanup service selection")
	}
	if err := r.requireCapability("runtime.container.remove"); err != nil {
		return err
	}
	observed, err := r.ObserveProject(ctx, project)
	if err != nil {
		return err
	}
	var selected *ProjectService
	for i := range observed {
		if observed[i].Service != service {
			continue
		}
		if selected != nil {
			return errors.New("remote cleanup service is ambiguous")
		}
		selected = &observed[i]
	}
	if selected == nil {
		return errors.New("remote cleanup service is absent")
	}
	return r.invoke(ctx, "runtime.container.remove", map[string]any{"resource_id": selected.ID, "force": force}, nil)
}
