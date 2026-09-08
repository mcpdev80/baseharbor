package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func (c Compose) ConfigProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	_, err := c.outputProjectFiles(ctx, project, workdir, composeFiles, "config", "--quiet")
	return err
}

func (c Compose) UpProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	_, err := c.outputProjectFiles(ctx, project, workdir, composeFiles, "up", "-d")
	return err
}

func (c Compose) DownProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) error {
	_, err := c.outputProjectFiles(ctx, project, workdir, composeFiles, "down")
	return err
}

func (c Compose) StatusProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) (string, error) {
	return c.outputProjectFiles(ctx, project, workdir, composeFiles, "ps")
}

func (c Compose) ExecProjectFiles(ctx context.Context, project, workdir, service string, composeFiles []string, args ...string) (string, error) {
	cmdArgs := append([]string{"exec", "-T", service}, args...)
	return c.outputProjectFiles(ctx, project, workdir, composeFiles, cmdArgs...)
}

func (c Compose) ServicesProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) ([]string, error) {
	out, err := c.outputProjectFiles(ctx, project, workdir, composeFiles, "config", "--services")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func (c Compose) RunningServicesProjectFiles(ctx context.Context, project, workdir string, composeFiles ...string) ([]string, error) {
	out, err := c.outputProjectFiles(ctx, project, workdir, composeFiles, "ps", "--services", "--status", "running")
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

func (c Compose) outputProjectFiles(ctx context.Context, project, workdir string, composeFiles []string, args ...string) (string, error) {
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
