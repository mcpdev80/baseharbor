package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrRuntimeNotFound = errors.New("docker compose or podman compose not found")

// Compose provides the small lifecycle surface BaseHarbor needs from a
// container runtime. Application code should not shell out to Docker/Podman
// directly.
type Compose struct {
	command string
	prefix  []string
}

func DetectCompose(ctx context.Context) (Compose, error) {
	if path, err := exec.LookPath("docker"); err == nil {
		cmd := exec.CommandContext(ctx, path, "compose", "version")
		if err := cmd.Run(); err == nil {
			return Compose{command: path, prefix: []string{"compose"}}, nil
		}
	}

	if path, err := exec.LookPath("podman"); err == nil {
		cmd := exec.CommandContext(ctx, path, "compose", "version")
		if err := cmd.Run(); err == nil {
			return Compose{command: path, prefix: []string{"compose"}}, nil
		}
	}

	return Compose{}, ErrRuntimeNotFound
}

func (c Compose) Up(ctx context.Context, composeFile, envFile string) error {
	return c.run(ctx, composeFile, envFile, "up", "-d")
}

func (c Compose) Down(ctx context.Context, composeFile, envFile string) error {
	return c.run(ctx, composeFile, envFile, "down")
}

func (c Compose) Status(ctx context.Context, composeFile, envFile string) (string, error) {
	return c.output(ctx, composeFile, envFile, "ps")
}

func (c Compose) Config(ctx context.Context, composeFile, envFile string) error {
	return c.run(ctx, composeFile, envFile, "config", "--quiet")
}

func (c Compose) run(ctx context.Context, composeFile, envFile string, args ...string) error {
	_, err := c.output(ctx, composeFile, envFile, args...)
	return err
}

func (c Compose) output(ctx context.Context, composeFile, envFile string, args ...string) (string, error) {
	if c.command == "" {
		return "", ErrRuntimeNotFound
	}

	fullArgs := append([]string{}, c.prefix...)
	fullArgs = append(fullArgs, "--project-name", "baseharbor", "--file", composeFile, "--env-file", envFile)
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, c.command, fullArgs...)
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
