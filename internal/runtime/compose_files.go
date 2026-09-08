package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
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
	_, err := c.outputProjectFilesEnv(ctx, project, workdir, nil, composeFiles, "config", "--quiet")
	return err
}

func (c Compose) ConfigProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) error {
	_, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "config", "--quiet")
	if err == nil {
		cacheProjectEnvironment(project, environment)
	}
	return err
}

func (c Compose) ConfigJSONProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) (string, error) {
	return c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "config", "--format", "json")
}

func (c Compose) UpProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	_, err := c.outputProjectFilesEnv(ctx, project, workdir, nil, composeFiles, "up", "-d")
	return err
}

func (c Compose) UpProjectFilesSelected(ctx context.Context, project, workdir string, environment map[string]string, services []string, composeFiles ...string) error {
	args := []string{"up", "-d"}
	if len(services) > 0 {
		args = append(args, "--no-deps")
		args = append(args, services...)
	}
	_, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, args...)
	return err
}

func (c Compose) DownProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	return c.DownProjectFilesEnv(ctx, project, workdir, takeProjectEnvironment(project), composeFiles...)
}

func (c Compose) DownProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) error {
	defer clearProjectEnvironment(project)
	_, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "down")
	return err
}

func (c Compose) StopProjectFilesSelected(ctx context.Context, project, workdir string, environment map[string]string, services []string, composeFiles ...string) error {
	if len(services) == 0 {
		return nil
	}
	args := []string{"rm", "-f", "-s"}
	args = append(args, services...)
	_, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, args...)
	return err
}

func (c Compose) StatusProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) (string, error) {
	return c.outputProjectFilesEnv(ctx, project, workdir, nil, composeFiles, "ps")
}

func (c Compose) ExecProjectFiles(ctx context.Context, project, workdir, service string, composeFiles []string, args ...string) (string, error) {
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
	out, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "config", "--services")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func (c Compose) RunningServicesProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) ([]string, error) {
	out, err := c.outputProjectFilesEnv(ctx, project, workdir, nil, composeFiles, "ps", "--services", "--status", "running")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func (c Compose) RunningServicesProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, composeFiles ...string) ([]string, error) {
	out, err := c.outputProjectFilesEnv(ctx, project, workdir, environment, composeFiles, "ps", "--services", "--status", "running")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
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
