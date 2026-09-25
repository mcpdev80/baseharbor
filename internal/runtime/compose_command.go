package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func (c Compose) directOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("runtime %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

func (c Compose) runProject(ctx context.Context, project, composeFile, envFile string, args ...string) error {
	_, err := c.outputProject(ctx, project, composeFile, envFile, args...)
	return err
}

func (c Compose) outputProject(ctx context.Context, project, composeFile, envFile string, args ...string) (string, error) {
	return c.outputProjectInput(ctx, project, composeFile, envFile, nil, args...)
}

func (c Compose) outputProjectInputProgress(ctx context.Context, project, composeFile, envFile string, input []byte, onProgress func(string), args ...string) (string, error) {
	if c.quadlet {
		return "", errors.New("internal error: Podman Quadlet runtime attempted Compose execution")
	}
	if c.command == "" {
		return "", ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" {
		return "", errors.New("compose project name is required")
	}

	fullArgs := append([]string{}, c.prefix...)
	fullArgs = append(fullArgs, "--project-name", project, "--file", composeFile, "--env-file", envFile)
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, c.command, fullArgs...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	var stdout bytes.Buffer
	progress := newComposeProgressCapture(onProgress)
	cmd.Stdout = &stdout
	cmd.Stderr = progress
	err := cmd.Run()
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

func (c Compose) outputProjectInput(ctx context.Context, project, composeFile, envFile string, input []byte, args ...string) (string, error) {
	if c.quadlet {
		return "", errors.New("internal error: Podman Quadlet runtime attempted Compose execution")
	}
	if c.command == "" {
		return "", ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" {
		return "", errors.New("compose project name is required")
	}

	fullArgs := append([]string{}, c.prefix...)
	fullArgs = append(fullArgs, "--project-name", project, "--file", composeFile, "--env-file", envFile)
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, c.command, fullArgs...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
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
