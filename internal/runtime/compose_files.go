package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

var projectEnvironmentCache = struct {
	sync.Mutex
	items map[string]map[string]string
}{items: map[string]map[string]string{}}

func cacheProjectEnvironment(project string, environment map[string]string) {
	if len(environment) == 0 {
		return
	}
	copy := make(map[string]string, len(environment))
	for key, value := range environment {
		copy[key] = value
	}
	projectEnvironmentCache.Lock()
	projectEnvironmentCache.items[project] = copy
	projectEnvironmentCache.Unlock()
}

func takeProjectEnvironment(project string) map[string]string {
	projectEnvironmentCache.Lock()
	defer projectEnvironmentCache.Unlock()
	environment := projectEnvironmentCache.items[project]
	delete(projectEnvironmentCache.items, project)
	return environment
}

func clearProjectEnvironment(project string) {
	projectEnvironmentCache.Lock()
	delete(projectEnvironmentCache.items, project)
	projectEnvironmentCache.Unlock()
}

func (c Compose) ConfigProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	return c.ConfigProjectFilesEnv(ctx, project, workdir, nil, composeFiles...)
}

func (c Compose) ConfigProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) error {
	if c.quadlet {
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
	_, quietErr := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "config", "--quiet")
	if quietErr != nil {
		if _, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "config"); err != nil {
			return fmt.Errorf("%v; plain config fallback: %w", quietErr, err)
		}
	}
	cacheProjectEnvironment(project, environment)
	return nil
}

func (c Compose) ConfigJSONProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) (string, error) {
	if c.quadlet {
		resolved, err := quadletResolveComposeFiles(workdir, composeFiles)
		if err != nil {
			return "", err
		}
		return RenderComposeProjectFilesJSON(resolved, environment)
	}
	rendered, jsonErr := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "config", "--format", "json")
	if jsonErr == nil {
		return rendered, nil
	}

	renderedYAML, yamlErr := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "config")
	if yamlErr != nil {
		return "", fmt.Errorf("%v; YAML fallback: %w", jsonErr, yamlErr)
	}

	var model any
	if err := yaml.Unmarshal([]byte(renderedYAML), &model); err != nil {
		return "", fmt.Errorf("decode Compose YAML fallback after %v: %w", jsonErr, yamlErr)
	}
	normalized, err := json.Marshal(model)
	if err != nil {
		return "", fmt.Errorf("encode Compose YAML fallback as JSON after %v: %w", jsonErr, err)
	}
	return string(normalized), nil
}

func (c Compose) UpProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	_, err := c.outputProjectFilesEnv(ctx, project, workdir, nil, composeFiles, "up", "-d")
	return err
}

func (c Compose) UpProjectFilesSelected(ctx context.Context, project, workdir string, environment map[string]string, services []string, composeFiles ...string) error {
	return c.UpProjectFilesSelectedProgress(ctx, project, workdir, environment, services, nil, composeFiles...)
}

func composeUpArgs(services []string) []string {
	args := []string{"up", "-d", "--build"}
	if len(services) > 0 {
		args = append(args, "--no-deps")
		args = append(args, services...)
	}
	return args
}

func (c Compose) UpProjectFilesSelectedProgress(ctx context.Context, project, workdir string, environment map[string]string, services []string, onProgress func(string), composeFiles ...string) error {
	if c.quadlet {
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
	args := composeUpArgs(services)
	_, err := c.outputProjectFilesEnvProgress(ctx, project, workdir, environment, composeFiles, onProgress, args...)
	return err
}

func (c Compose) DownProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	return c.DownProjectFilesEnv(ctx, project, workdir, takeProjectEnvironment(project), composeFiles...)
}

func (c Compose) DownProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) error {
	defer clearProjectEnvironment(project)
	if c.quadlet {
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
	_, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "down")
	return err
}

func (c Compose) StopProjectFilesSelected(ctx context.Context, project, workdir string, environment map[string]string, services []string, composeFiles ...string) error {
	if len(services) == 0 {
		return nil
	}
	if c.quadlet {
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

	selected := make(map[string]struct{}, len(services))
	for _, service := range services {
		selected[service] = struct{}{}
	}

	args := []string{"stop"}
	args = append(args, services...)
	if _, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, args...); err != nil {
		containers, listErr := c.ListComposeContainers(ctx)
		if listErr != nil {
			return fmt.Errorf("%v; engine-level selected-service fallback: %w", err, listErr)
		}
		for _, container := range containers {
			if container.Project != project {
				continue
			}
			if _, ok := selected[container.Service]; !ok {
				continue
			}
			if _, stopErr := c.directOutput(ctx, "container", "stop", container.Name); stopErr != nil {
				return fmt.Errorf("%v; stop selected service %s via runtime engine: %w", err, container.Service, stopErr)
			}
		}
	}

	containers, err := c.ListComposeContainers(ctx)
	if err != nil {
		return fmt.Errorf("list selected workload containers after stop: %w", err)
	}
	for _, container := range containers {
		if container.Project != project {
			continue
		}
		if _, ok := selected[container.Service]; !ok {
			continue
		}
		if _, removeErr := c.directOutput(ctx, "container", "rm", container.Name); removeErr != nil {
			return fmt.Errorf("remove stopped selected service %s via runtime engine: %w", container.Service, removeErr)
		}
	}
	return nil
}

func (c Compose) StatusProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) (string, error) {
	if c.quadlet {
		return c.StatusProject(ctx, project, "", "")
	}
	return c.outputProjectFilesEnv(ctx, project, workdir, nil, composeFiles, "ps")
}

func (c Compose) ExecProjectFiles(ctx context.Context, project, workdir, service string, composeFiles []string, args ...string) (string, error) {
	if c.quadlet {
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
		return quadletExec(ctx, c.command, container, nil, args...)
	}
	cmdArgs := append([]string{"exec", "-T", service}, args...)
	return c.outputProjectFilesEnv(ctx, project, workdir, nil, composeFiles, cmdArgs...)
}

func (c Compose) ServicesProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) ([]string, error) {
	out, err := c.outputProjectFilesEnv(ctx, project, workdir, nil, composeFiles, "config", "--services")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func (c Compose) ServicesProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) ([]string, error) {
	if c.quadlet {
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
	out, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "config", "--services")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func (c Compose) RunningServicesProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) ([]string, error) {
	return c.RunningServicesProjectFilesEnv(ctx, project, workdir, nil, composeFiles...)
}

func (c Compose) RunningServicesProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) ([]string, error) {
	states, err := c.ServiceStatesProjectFilesEnv(ctx, project, workdir, environment, composeFiles...)
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

func nonEmptyLines(out string) []string {
	var values []string
	for _, line := range strings.Split(out, "\n") {
		if value := strings.TrimSpace(line); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func mergeProcessEnvironment(overrides map[string]string) ([]string, error) {
	if len(overrides) == 0 {
		return os.Environ(), nil
	}
	for key, value := range overrides {
		if key == "" || strings.ContainsRune(key, '=') || strings.ContainsRune(value, 0) {
			return nil, errors.New("invalid compose process environment")
		}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		env = append(env, entry)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env, nil
}

func (c Compose) outputProjectFilesEnvProgress(ctx context.Context, project, workdir string, environment map[string]string, composeFiles []string, onProgress func(string), args ...string) (string, error) {
	if c.command == "" {
		return "", ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" {
		return "", errors.New("compose project name is required")
	}
	if len(composeFiles) == 0 {
		return "", errors.New("at least one compose file is required")
	}

	fullArgs := append([]string{}, c.prefix...)
	fullArgs = append(fullArgs, "--project-name", project)
	for _, file := range composeFiles {
		if strings.TrimSpace(file) == "" {
			return "", errors.New("compose file path is empty")
		}
		fullArgs = append(fullArgs, "--file", file)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, c.command, fullArgs...)
	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = workdir
	}
	var err error
	cmd.Env, err = mergeProcessEnvironment(environment)
	if err != nil {
		return "", err
	}

	var stdout bytes.Buffer
	progress := newComposeProgressCapture(onProgress)
	cmd.Stdout = &stdout
	cmd.Stderr = progress
	err = cmd.Run()
	progress.Flush()
	if err != nil {
		message := strings.TrimSpace(progress.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("compose %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

func (c Compose) outputProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles []string, args ...string) (string, error) {
	if c.command == "" {
		return "", ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" {
		return "", errors.New("compose project name is required")
	}
	if len(composeFiles) == 0 {
		return "", errors.New("at least one compose file is required")
	}

	fullArgs := append([]string{}, c.prefix...)
	fullArgs = append(fullArgs, "--project-name", project)
	for _, file := range composeFiles {
		if strings.TrimSpace(file) == "" {
			return "", errors.New("compose file path is empty")
		}
		fullArgs = append(fullArgs, "--file", file)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, c.command, fullArgs...)
	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = workdir
	}
	var err error
	cmd.Env, err = mergeProcessEnvironment(environment)
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("compose %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}
