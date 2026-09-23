package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// RunProjectFilesEnv runs a workload command with caller-owned streams.
// Docker uses Compose; Podman resolves the same workload through Quadlet-owned
// containers without falling back to podman compose.
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

	if c.quadlet {
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
				cmd := exec.CommandContext(ctx, c.command, logArgs...)
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
			cmd := exec.CommandContext(ctx, c.command, cmdArgs...)
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
