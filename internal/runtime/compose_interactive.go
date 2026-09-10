package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// RunProjectFilesEnv runs a Compose project command with caller-owned streams.
// It is used for interactive developer actions such as logs and workload
// shells without exposing container names to the caller.
func (c Compose) RunProjectFilesEnv(ctx context.Context, project, workdir string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer, composeFiles []string, args ...string) error {
	if c.command == "" {
		return ErrRuntimeNotFound
	}
	if strings.TrimSpace(project) == "" {
		return errors.New("compose project name is required")
	}
	if len(composeFiles) == 0 {
		return errors.New("at least one compose file is required")
	}

	fullArgs := append([]string{}, c.prefix...)
	fullArgs = append(fullArgs, "--project-name", project)
	for _, file := range composeFiles {
		if strings.TrimSpace(file) == "" {
			return errors.New("compose file path is empty")
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
		return err
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("compose %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
