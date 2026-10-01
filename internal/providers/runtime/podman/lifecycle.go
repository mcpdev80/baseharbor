package podman

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// PodmanProvider is the first-party Podman Runtime Provider. Repository Compose
// remains an input/source format, while realization is owned by Quadlet and the
// user systemd manager. No portable Core code needs to know that distinction.

func (p PodmanProvider) LogCollectionMode() LogCollectionMode { return LogCollectionJournald }
func (p PodmanProvider) VerifyProjectServiceLogCollection(ctx context.Context, project, service, expectedTag string) error {
	driver, err := p.ProjectServiceLogDriver(ctx, project, service)
	if err != nil {
		return err
	}
	if driver != "journald" {
		return fmt.Errorf("runtime log driver for %s/%s: got %q, want journald", project, service, driver)
	}
	return nil
}

func (p PodmanProvider) Up(ctx context.Context, composeFile, envFile string) error {
	return p.UpProject(ctx, "baseharbor", composeFile, envFile)
}

func (p PodmanProvider) Down(ctx context.Context, composeFile, envFile string) error {
	return p.DownProject(ctx, "baseharbor", composeFile, envFile)
}

func (p PodmanProvider) Status(ctx context.Context, composeFile, envFile string) (string, error) {
	return p.StatusProject(ctx, "baseharbor", composeFile, envFile)
}

func (p PodmanProvider) Config(ctx context.Context, composeFile, envFile string) error {
	return p.ConfigProject(ctx, "baseharbor", composeFile, envFile)
}

func (p PodmanProvider) UpProject(ctx context.Context, project, composeFile, envFile string) error {
	return p.UpProjectProgress(ctx, project, composeFile, envFile, nil)
}

func (p PodmanProvider) UpProjectProgress(ctx context.Context, project, composeFile, envFile string, onProgress func(string)) error {
	q, err := quadletRenderProject(composeFile, envFile, project)
	if err != nil {
		return err
	}
	if onProgress != nil {
		onProgress("rendered Podman Quadlet runtime")
	}
	if err := quadletStartProject(ctx, q, nil); err != nil {
		return err
	}
	if onProgress != nil {
		onProgress("started Podman Quadlet services")
	}
	return nil
}

func (p PodmanProvider) DownProject(ctx context.Context, project, composeFile, envFile string) error {
	q, err := quadletRenderProject(composeFile, envFile, project)
	if err != nil {
		return err
	}
	return quadletRemoveProject(ctx, q, false)
}

func (p PodmanProvider) StopProject(ctx context.Context, project, composeFile, envFile string) error {
	q, err := quadletRenderProject(composeFile, envFile, project)
	if err != nil {
		return err
	}
	return quadletStopProject(ctx, q, nil)
}

func (p PodmanProvider) DownProjectRemoveOrphans(ctx context.Context, project, composeFile, envFile string) error {
	return p.DownProject(ctx, project, composeFile, envFile)
}

func (p PodmanProvider) DestroyProject(ctx context.Context, project, composeFile, envFile string) error {
	q, err := quadletRenderProject(composeFile, envFile, project)
	if err != nil {
		return err
	}
	return quadletRemoveProject(ctx, q, true)
}

func (p PodmanProvider) DestroyProjectRemoveOrphans(ctx context.Context, project, composeFile, envFile string) error {
	return p.DestroyProject(ctx, project, composeFile, envFile)
}

func (p PodmanProvider) StatusProject(ctx context.Context, project, composeFile, envFile string) (string, error) {
	containers, err := p.ListRuntimeContainers(ctx)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, container := range containers {
		if container.Project == project {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%t", container.Service, container.Name, container.Running))
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

func (p PodmanProvider) LogsProject(ctx context.Context, project, composeFile, envFile string, services ...string) (string, error) {
	q, err := quadletRenderProject(composeFile, envFile, project)
	if err != nil {
		return "", err
	}
	return quadletLogs(ctx, p.CommandPath(), q, services)
}

func (p PodmanProvider) DiagnosticsProject(ctx context.Context, project, composeFile, envFile string) string {
	q, renderErr := quadletRenderProject(composeFile, envFile, project)
	status, statusErr := p.StatusProject(ctx, project, composeFile, envFile)
	logs, logsErr := p.LogsProject(ctx, project, composeFile, envFile)
	var b strings.Builder
	if renderErr != nil {
		fmt.Fprintf(&b, "Quadlet render failed: %v\n", renderErr)
	}
	if statusErr != nil {
		fmt.Fprintf(&b, "Quadlet status failed: %v\n", statusErr)
	} else {
		fmt.Fprintf(&b, "Quadlet status:\n%s\n", status)
	}
	if renderErr == nil {
		units := make([]string, 0, len(q.ServiceUnits))
		for _, unit := range q.ServiceUnits {
			units = append(units, unit)
		}
		sort.Strings(units)
		for _, unit := range units {
			unitStatus, _ := quadletSystemctlCombined(ctx, "status", "--no-pager", "--full", unit)
			if strings.TrimSpace(unitStatus) != "" {
				fmt.Fprintf(&b, "Quadlet unit %s:\n%s\n", unit, strings.TrimSpace(unitStatus))
			}
		}
		services := make([]string, 0, len(q.Containers))
		for service := range q.Containers {
			services = append(services, service)
		}
		sort.Strings(services)
		for _, service := range services {
			container := q.Containers[service]
			cmd := exec.CommandContext(ctx, p.CommandPath(), "inspect", "--format", "{{json .State.Health}}", container)
			cmd.Env = runtimeCommandEnv(p.CommandPath())
			if health, err := cmd.CombinedOutput(); err == nil && strings.TrimSpace(string(health)) != "" {
				fmt.Fprintf(&b, "Podman health %s:\n%s\n", service, strings.TrimSpace(string(health)))
			}
		}
	}
	if logsErr != nil {
		fmt.Fprintf(&b, "Quadlet logs failed: %v\n", logsErr)
	} else {
		fmt.Fprintf(&b, "Quadlet logs:\n%s\n", logs)
	}
	return strings.TrimSpace(b.String())
}

func (p PodmanProvider) ConfigProject(ctx context.Context, project, composeFile, envFile string) error {
	q, err := quadletRenderProject(composeFile, envFile, project)
	if err != nil {
		return err
	}
	return quadletValidateProject(ctx, q)
}

func (p PodmanProvider) ExecProject(ctx context.Context, project, composeFile, envFile, service string, args ...string) (string, error) {
	q, err := quadletRenderProject(composeFile, envFile, project)
	if err != nil {
		return "", err
	}
	container, ok := q.Containers[service]
	if !ok {
		return "", fmt.Errorf("Quadlet service %q is not part of project %s", service, project)
	}
	return quadletExec(ctx, p.CommandPath(), container, nil, args...)
}

func (p PodmanProvider) ExecProjectInput(ctx context.Context, project, composeFile, envFile string, input []byte, service string, args ...string) (string, error) {
	q, err := quadletRenderProject(composeFile, envFile, project)
	if err != nil {
		return "", err
	}
	container, ok := q.Containers[service]
	if !ok {
		return "", fmt.Errorf("Quadlet service %q is not part of project %s", service, project)
	}
	return quadletExec(ctx, p.CommandPath(), container, input, args...)
}

func (p PodmanProvider) ConfigProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	return p.ConfigProjectFilesEnv(ctx, project, workdir, nil, composeFiles...)
}

func (p PodmanProvider) ConfigProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) error {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project)
	if err != nil {
		return err
	}
	if err := quadletValidateProject(ctx, q); err != nil {
		return err
	}
	cacheProjectEnvironment(project, environment)
	return nil
}

func (p PodmanProvider) ConfigJSONProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) (string, error) {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return "", err
	}
	return RenderComposeProjectFilesJSON(resolved, environment)
}

func (p PodmanProvider) UpProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	return p.UpProjectFilesSelected(ctx, project, workdir, nil, nil, composeFiles...)
}

func (p PodmanProvider) UpProjectFilesSelected(ctx context.Context, project, workdir string, environment map[string]string, services []string, composeFiles ...string) error {
	return p.UpProjectFilesSelectedProgress(ctx, project, workdir, environment, services, nil, composeFiles...)
}

func (p PodmanProvider) BuildProjectFilesSelectedProgress(ctx context.Context, project, workdir string, environment map[string]string, services []string, onProgress func(string), composeFiles ...string) error {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project, services...)
	if err != nil {
		return err
	}
	if onProgress != nil {
		onProgress("rebuilding changed Podman Quadlet workload image")
	}
	if err := quadletBuildProject(ctx, q, services); err != nil {
		return err
	}
	cacheProjectEnvironment(project, environment)
	return nil
}

func (p PodmanProvider) UpProjectFilesSelectedForceRecreateNoBuild(ctx context.Context, project, workdir string, environment map[string]string, services []string, composeFiles ...string) error {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project, services...)
	if err != nil {
		return err
	}
	if err := quadletForceRestartProjectNoBuild(ctx, q, services); err != nil {
		return err
	}
	cacheProjectEnvironment(project, environment)
	return nil
}

func (p PodmanProvider) UpProjectFilesSelectedNoBuildProgress(ctx context.Context, project, workdir string, environment map[string]string, services []string, onProgress func(string), composeFiles ...string) error {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project, services...)
	if err != nil {
		return err
	}
	if onProgress != nil {
		onProgress("rendered Podman Quadlet workload")
	}
	if err := quadletStartProjectNoBuild(ctx, q, services); err != nil {
		return err
	}
	cacheProjectEnvironment(project, environment)
	if onProgress != nil {
		onProgress("started Podman Quadlet workload")
	}
	return nil
}

func (p PodmanProvider) UpProjectFilesSelectedProgress(ctx context.Context, project, workdir string, environment map[string]string, services []string, onProgress func(string), composeFiles ...string) error {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project, services...)
	if err != nil {
		return err
	}
	if onProgress != nil {
		onProgress("rendered Podman Quadlet workload")
	}
	if err := quadletStartProject(ctx, q, services); err != nil {
		return err
	}
	cacheProjectEnvironment(project, environment)
	if onProgress != nil {
		onProgress("started Podman Quadlet workload")
	}
	return nil
}

func (p PodmanProvider) DownProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	return p.DownProjectFilesEnv(ctx, project, workdir, takeProjectEnvironment(project), composeFiles...)
}

func (p PodmanProvider) DownProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) error {
	defer clearProjectEnvironment(project)
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project)
	if err != nil {
		return err
	}
	return quadletRemoveProject(ctx, q, false)
}

func (p PodmanProvider) StopProjectFilesSelected(ctx context.Context, project, workdir string, environment map[string]string, services []string, composeFiles ...string) error {
	if len(services) == 0 {
		return nil
	}
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project)
	if err != nil {
		return err
	}
	return quadletStopProject(ctx, q, services)
}

func (p PodmanProvider) StatusProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) (string, error) {
	return p.StatusProject(ctx, project, "", "")
}

func (p PodmanProvider) ExecProjectFiles(ctx context.Context, project, workdir, service string, composeFiles []string, args ...string) (string, error) {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return "", err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", nil, project)
	if err != nil {
		return "", err
	}
	container, ok := q.Containers[service]
	if !ok {
		return "", fmt.Errorf("Quadlet service %q is not part of project %s", service, project)
	}
	return quadletExec(ctx, p.CommandPath(), container, nil, args...)
}

func (p PodmanProvider) ExecProjectFilesInput(ctx context.Context, project, workdir, service string, composeFiles []string, input []byte, args ...string) (string, error) {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return "", err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", nil, project)
	if err != nil {
		return "", err
	}
	container, ok := q.Containers[service]
	if !ok {
		return "", fmt.Errorf("Quadlet service %q is not part of project %s", service, project)
	}
	return quadletExec(ctx, p.CommandPath(), container, input, args...)
}

func (p PodmanProvider) ServicesProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) ([]string, error) {
	return p.ServicesProjectFilesEnv(ctx, project, workdir, nil, composeFiles...)
}

func (p PodmanProvider) ServicesProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) ([]string, error) {
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return nil, err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project)
	if err != nil {
		return nil, err
	}
	services := make([]string, 0, len(q.ServiceUnits))
	for service := range q.ServiceUnits {
		services = append(services, service)
	}
	sort.Strings(services)
	return services, nil
}

func (p PodmanProvider) RunningServicesProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) ([]string, error) {
	return p.RunningServicesProjectFilesEnv(ctx, project, workdir, nil, composeFiles...)
}

func (p PodmanProvider) RunningServicesProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) ([]string, error) {
	states, err := p.ServiceStatesProjectFilesEnv(ctx, project, workdir, environment, composeFiles...)
	if err != nil {
		return nil, err
	}
	services := make([]string, 0, len(states))
	for _, state := range states {
		if state.Ready() {
			services = append(services, state.Service)
		}
	}
	return services, nil
}

func (p PodmanProvider) ServiceStatesProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) ([]ServiceState, error) {
	return serviceStatesFromRuntimeLabels(ctx, p.Compose, project)
}

func (p PodmanProvider) RunProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer, composeFiles []string, args ...string) error {
	if p.CommandPath() == "" {
		return ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" {
		return errors.New("project name is required")
	}
	if len(composeFiles) == 0 {
		return errors.New("at least one workload source file is required")
	}
	resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
	if err != nil {
		return err
	}
	q, err := RenderComposeProjectFilesQuadletsEnv(resolved, "", environment, project)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return errors.New("workload command is required")
	}

	switch args[0] {
	case "logs":
		follow := false
		var services []string
		for _, arg := range args[1:] {
			switch arg {
			case "--follow", "-f":
				follow = true
			default:
				if strings.HasPrefix(arg, "-") {
					return fmt.Errorf("unsupported Podman Quadlet logs option %q", arg)
				}
				services = append(services, arg)
			}
		}
		if len(services) == 0 {
			for service := range q.Containers {
				services = append(services, service)
			}
		}
		sort.Strings(services)
		for _, service := range services {
			container, ok := q.Containers[service]
			if !ok {
				return fmt.Errorf("Quadlet service %q is not part of project %s", service, project)
			}
			logArgs := []string{"logs"}
			if follow {
				logArgs = append(logArgs, "--follow")
			} else {
				logArgs = append(logArgs, "--tail", "120")
			}
			logArgs = append(logArgs, container)
			cmd := exec.CommandContext(ctx, p.CommandPath(), logArgs...)
			cmd.Env = runtimeCommandEnv(p.CommandPath())
			cmd.Stdin = stdin
			cmd.Stdout = stdout
			cmd.Stderr = stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("podman logs %s: %w", service, err)
			}
		}
		return nil
	case "exec":
		execArgs := args[1:]
		if len(execArgs) > 0 && execArgs[0] == "-T" {
			execArgs = execArgs[1:]
		}
		if len(execArgs) < 2 {
			return errors.New("exec requires SERVICE and COMMAND")
		}
		service := execArgs[0]
		container, ok := q.Containers[service]
		if !ok {
			return fmt.Errorf("Quadlet service %q is not part of project %s", service, project)
		}
		cmdArgs := []string{"exec", "-i", container}
		cmdArgs = append(cmdArgs, execArgs[1:]...)
		cmd := exec.CommandContext(ctx, p.CommandPath(), cmdArgs...)
		cmd.Env = runtimeCommandEnv(p.CommandPath())
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("podman exec %s: %w", service, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported Podman Quadlet workload command %q", args[0])
	}
}

func (p PodmanProvider) RunningServicesProject(ctx context.Context, project, composeFile, envFile string) ([]string, error) {
	if p.CommandPath() == "" {
		return nil, ErrRuntimeNotFound
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, errors.New("project name is required")
	}
	var moduleServices map[string]struct{}
	if consolidatedProject(project) && strings.TrimSpace(composeFile) != "" {
		moduleServices = map[string]struct{}{}
		q, err := quadletRenderProject(composeFile, envFile, project)
		if err != nil {
			return nil, err
		}
		for service := range q.ServiceUnits {
			moduleServices[service] = struct{}{}
		}
	}
	containers, err := p.ListRuntimeContainers(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var services []string
	for _, container := range containers {
		if container.Project != project || !container.Running {
			continue
		}
		if moduleServices != nil {
			if _, ok := moduleServices[container.Service]; !ok {
				continue
			}
		}
		if _, ok := seen[container.Service]; ok {
			continue
		}
		seen[container.Service] = struct{}{}
		services = append(services, container.Service)
	}
	sort.Strings(services)
	return services, nil
}

func composeContainerServiceFromExpectedName(project, name string) (string, bool) {
	project = strings.TrimSpace(project)
	name = strings.TrimSpace(name)
	prefix := project + "-"
	if project == "" || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, "-1") {
		return "", false
	}
	service := strings.TrimSuffix(strings.TrimPrefix(name, prefix), "-1")
	if strings.TrimSpace(service) == "" {
		return "", false
	}
	return service, true
}

func (p PodmanProvider) resolveOwnedContainerResourceName(ctx context.Context, project, requested string) (string, bool, error) {
	requested = strings.TrimSpace(requested)
	exists, err := quadletRuntimeResourceExists(ctx, "container", requested)
	if err != nil {
		return "", false, err
	}
	if exists {
		return requested, true, nil
	}
	service, ok := composeContainerServiceFromExpectedName(project, requested)
	if !ok {
		return "", false, nil
	}
	containers, err := p.ListRuntimeContainers(ctx)
	if err != nil {
		return "", false, err
	}
	var actual string
	for _, container := range containers {
		if container.Project != project || container.Service != service {
			continue
		}
		if actual != "" && actual != container.Name {
			return "", false, fmt.Errorf("multiple Podman containers match owned service %s/%s", project, service)
		}
		actual = container.Name
	}
	if actual == "" {
		return "", false, nil
	}
	return actual, true, nil
}

func (p PodmanProvider) InspectProjectResource(ctx context.Context, project string, resource ProjectResource) (bool, error) {
	existing, err := p.InspectProjectResources(ctx, project, []ProjectResource{resource})
	if err != nil {
		return false, err
	}
	return len(existing) == 1, nil
}

func (p PodmanProvider) InspectProjectResources(ctx context.Context, project string, resources []ProjectResource) ([]ProjectResource, error) {
	if p.CommandPath() == "" {
		return nil, ErrRuntimeNotFound
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, errors.New("project is required")
	}
	if len(resources) == 0 {
		return nil, nil
	}
	found := map[string]struct{}{}
	for _, resource := range resources {
		name := strings.TrimSpace(resource.Name)
		if name == "" {
			return nil, errors.New("resource name is required")
		}
		switch resource.Kind {
		case "container", "network", "volume":
		default:
			return nil, fmt.Errorf("unsupported runtime resource kind %q", resource.Kind)
		}
		actualName := name
		var exists bool
		var err error
		if resource.Kind == "container" {
			actualName, exists, err = p.resolveOwnedContainerResourceName(ctx, project, name)
		} else {
			exists, err = quadletRuntimeResourceExists(ctx, resource.Kind, name)
		}
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		var template string
		switch resource.Kind {
		case "container":
			template = `{{.Name}}|{{ index .Config.Labels "com.docker.compose.project" }}|{{ index .Config.Labels "io.podman.compose.project" }}`
		default:
			template = `{{.Name}}|{{ index .Labels "com.docker.compose.project" }}|{{ index .Labels "io.podman.compose.project" }}`
		}
		out, err := p.DirectOutput(ctx, resource.Kind, "inspect", "--format", template, actualName)
		if err != nil {
			return nil, fmt.Errorf("inspect %s ownership: %w", resource.Kind, err)
		}
		parts := strings.Split(strings.TrimSpace(out), "|")
		if len(parts) != 3 {
			return nil, fmt.Errorf("inspect %s %s ownership returned invalid result", resource.Kind, name)
		}
		owner := firstRuntimeLabel(parts[1], parts[2])
		if owner != project {
			return nil, fmt.Errorf("%w: %s %s is not owned by project %s", ErrResourceOwnership, resource.Kind, name, project)
		}
		found[resource.Kind+"\x00"+name] = struct{}{}
	}
	existing := make([]ProjectResource, 0, len(found))
	for _, resource := range resources {
		if _, ok := found[resource.Kind+"\x00"+strings.TrimSpace(resource.Name)]; ok {
			existing = append(existing, resource)
		}
	}
	return existing, nil
}

func podmanSystemdUnitLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "<no value>" {
		return ""
	}
	return value
}

func (p PodmanProvider) StopOwnedProjectContainers(ctx context.Context, project string) error {
	project = strings.TrimSpace(project)
	if project == "" {
		return errors.New("project is required")
	}
	containers, err := p.ListRuntimeContainers(ctx)
	if err != nil {
		return err
	}

	var units []string
	var direct []RuntimeContainer
	seenUnits := map[string]struct{}{}
	for _, container := range containers {
		if container.Project != project || !container.Running {
			continue
		}
		unitOut, inspectErr := p.DirectOutput(ctx,
			"container", "inspect", "--format",
			`{{ index .Config.Labels "PODMAN_SYSTEMD_UNIT" }}`,
			container.Name,
		)
		if inspectErr != nil {
			return fmt.Errorf("inspect Quadlet unit for %s/%s (%s): %w", project, container.Service, container.Name, inspectErr)
		}
		unit := podmanSystemdUnitLabel(unitOut)
		if unit == "" {
			direct = append(direct, container)
			continue
		}
		if _, seen := seenUnits[unit]; seen {
			continue
		}
		seenUnits[unit] = struct{}{}
		units = append(units, unit)
	}
	sort.Strings(units)

	if len(units) > 0 {
		if _, stopErr := quadletSystemctlCombined(ctx, append([]string{"stop"}, units...)...); stopErr != nil {
			return fmt.Errorf("stop owned Quadlet services for project %s: %w", project, stopErr)
		}
	}
	for _, container := range direct {
		if _, stopErr := p.DirectOutput(ctx, "container", "stop", container.Name); stopErr != nil {
			return fmt.Errorf("stop owned Podman container %s/%s (%s): %w", project, container.Service, container.Name, stopErr)
		}
	}

	remaining, err := p.ListRuntimeContainers(ctx)
	if err != nil {
		return err
	}
	for _, container := range remaining {
		if container.Project == project && container.Running {
			return fmt.Errorf("verify owned project stop: container %s/%s (%s) is still running", project, container.Service, container.Name)
		}
	}
	return nil
}

func (p PodmanProvider) DestroyOwnedProjectResources(ctx context.Context, project string, resources []ProjectResource) error {
	existing, err := p.InspectProjectResources(ctx, project, resources)
	if err != nil {
		return err
	}

	var containerUnits []string
	seenUnits := map[string]struct{}{}
	for _, resource := range existing {
		if resource.Kind != "container" {
			continue
		}
		actualName, exists, resolveErr := p.resolveOwnedContainerResourceName(ctx, project, resource.Name)
		if resolveErr != nil {
			return resolveErr
		}
		if !exists {
			continue
		}
		unitOut, inspectErr := p.DirectOutput(ctx,
			"container", "inspect", "--format",
			`{{ index .Config.Labels "PODMAN_SYSTEMD_UNIT" }}`,
			actualName,
		)
		if inspectErr != nil {
			return fmt.Errorf("inspect Quadlet unit for owned container %s: %w", resource.Name, inspectErr)
		}
		unit := podmanSystemdUnitLabel(unitOut)
		if unit == "" {
			continue
		}
		if _, seen := seenUnits[unit]; seen {
			continue
		}
		seenUnits[unit] = struct{}{}
		containerUnits = append(containerUnits, unit)
	}
	sort.Strings(containerUnits)
	if len(containerUnits) > 0 {
		if _, stopErr := quadletSystemctlCombined(ctx, append([]string{"stop"}, containerUnits...)...); stopErr != nil {
			return fmt.Errorf("stop owned Quadlet services before resource removal for project %s: %w", project, stopErr)
		}
	}

	for _, kind := range []string{"container", "network", "volume"} {
		for _, resource := range existing {
			if resource.Kind != kind {
				continue
			}
			actualName := resource.Name
			if kind == "container" {
				resolvedName, exists, resolveErr := p.resolveOwnedContainerResourceName(ctx, project, resource.Name)
				if resolveErr != nil {
					return resolveErr
				}
				if !exists {
					continue
				}
				actualName = resolvedName
			}
			args := []string{kind, "rm", actualName}
			if kind == "container" {
				args = []string{"container", "rm", "-f", actualName}
			}
			if _, err := p.DirectOutput(ctx, args...); err != nil {
				if kind == "network" && podmanNetworkHasActiveConsumers(err) {
					continue
				}
				remaining, inspectErr := p.InspectProjectResources(ctx, project, []ProjectResource{resource})
				if inspectErr == nil && len(remaining) == 0 {
					continue
				}
				if inspectErr != nil {
					return fmt.Errorf("remove owned %s %s: %w (post-remove verification failed: %v)", kind, resource.Name, err, inspectErr)
				}
				return fmt.Errorf("remove owned %s %s: %w", kind, resource.Name, err)
			}
		}
	}
	return nil
}

func podmanNetworkHasActiveConsumers(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "network is being used") ||
		strings.Contains(message, "has associated containers") ||
		strings.Contains(message, "active endpoints")
}

func (p PodmanProvider) ContainerLogConfigProjectService(context.Context, string, string) (string, string, error) {
	return "", "", nil
}

func (p PodmanProvider) ProjectServiceLogDriver(ctx context.Context, project, service string) (string, error) {
	project = strings.TrimSpace(project)
	service = strings.TrimSpace(service)
	if project == "" || service == "" {
		return "", errors.New("project and service are required")
	}
	containers, err := p.ListRuntimeContainers(ctx)
	if err != nil {
		return "", err
	}
	for _, container := range containers {
		if container.Project == project && container.Service == service && container.Running {
			return "journald", nil
		}
	}
	return "", fmt.Errorf("running service %s/%s was not found", project, service)
}

func (p PodmanProvider) providerSourceFiles(workdir string, composeFiles []string) ([]string, error) {
	return quadletResolveComposeFiles(workdir, composeFiles)
}

func (p PodmanProvider) workdirFor(composeFile string) string {
	if strings.TrimSpace(composeFile) == "" {
		return ""
	}
	return filepath.Dir(composeFile)
}
