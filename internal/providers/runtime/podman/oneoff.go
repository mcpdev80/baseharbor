package podman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type nativeOneOffRequest struct {
	service, user, entrypoint string
	volumes, command          []string
}

// Recovery tools need a disposable native container, not a persistent
// systemd unit. Admit only the bounded Compose run contract used by callers;
// dependencies and published ports are intentionally not started.
func parseNativeOneOff(args []string) (nativeOneOffRequest, error) {
	var r nativeOneOffRequest
	rm, noDeps := false, false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		option := args[0]
		args = args[1:]
		switch option {
		case "--rm":
			rm = true
		case "--no-deps":
			noDeps = true
		case "--user", "--entrypoint", "-v", "--volume":
			if len(args) == 0 || args[0] == "" {
				return r, errors.New("missing native one-off option value")
			}
			value := args[0]
			args = args[1:]
			switch option {
			case "--user":
				r.user = value
			case "--entrypoint":
				r.entrypoint = value
			default:
				r.volumes = append(r.volumes, value)
			}
		default:
			return r, fmt.Errorf("unsupported native one-off option %q", option)
		}
	}
	if !rm || !noDeps || len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return r, errors.New("native one-off requires --rm --no-deps SERVICE")
	}
	r.service, r.command = args[0], args[1:]
	return r, nil
}

func nativeOneOffArgs(project, composePath string, model quadletComposeProject, r nativeOneOffRequest) ([]string, map[string]string, []ProjectResource, error) {
	service, ok := model.Services[r.service]
	if !ok || strings.TrimSpace(service.Image) == "" {
		return nil, nil, nil, errors.New("native one-off requires a declared service image")
	}
	args := []string{"run", "--rm", "-i", "--label", "com.docker.compose.project=" + project, "--label", "com.docker.compose.service=" + r.service}
	if service.ReadOnly {
		args = append(args, "--read-only")
	}
	for _, cap := range service.CapDrop {
		args = append(args, "--cap-drop", cap)
	}
	for _, cap := range service.CapAdd {
		args = append(args, "--cap-add", cap)
	}
	for _, option := range service.SecurityOpt {
		args = append(args, "--security-opt", option)
	}
	for _, tmpfs := range service.Tmpfs {
		args = append(args, "--tmpfs", tmpfs)
	}
	user := r.user
	if user == "" {
		user = service.User
	}
	if user != "" {
		args = append(args, "--user", user)
	}
	// Preserve host-owned private bind files for non-root one-off tools. This
	// namespace mapping does not grant capabilities or relax file permissions.
	if os.Getuid() != 0 && user != "" {
		args = append(args, "--userns=keep-id")
	}
	var resources []ProjectResource
	networks := service.Networks.Names
	if len(networks) == 0 {
		networks = []string{"default"}
	}
	for _, name := range networks {
		resource, declared := model.Networks[name]
		if !declared {
			return nil, nil, nil, errors.New("undeclared native one-off network")
		}
		actual := resource.Name
		if actual == "" {
			if resource.External {
				actual = name
			} else {
				actual = project + "_" + name
			}
		}
		if !resource.External {
			resources = append(resources, ProjectResource{Kind: "network", Name: actual})
		}
		args = append(args, "--network", actual)
	}
	// An override replaces the inherited mount at the same container target.
	mounts := map[string]string{}
	for _, mount := range append(append([]string(nil), service.Volumes...), r.volumes...) {
		parts := strings.Split(mount, ":")
		if len(parts) < 2 || len(parts) > 3 || !strings.HasPrefix(parts[1], "/") {
			return nil, nil, nil, errors.New("invalid native one-off mount")
		}
		if volume, ok := model.Volumes[parts[0]]; ok {
			actual := volume.Name
			if actual == "" {
				if volume.External {
					actual = parts[0]
				} else {
					actual = project + "_" + parts[0]
				}
			}
			if !volume.External {
				resources = append(resources, ProjectResource{Kind: "volume", Name: actual})
			}
			parts[0] = actual
			mount = strings.Join(parts, ":")
		} else {
			var err error
			mount, err = renderQuadletVolumeMount(composePath, project, nil, mount)
			if err != nil {
				return nil, nil, nil, err
			}
		}
		mounts[parts[1]] = mount
	}
	var targets []string
	for target := range mounts {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		args = append(args, "--volume", mounts[target])
	}
	if len(service.Secrets) > 0 {
		return nil, nil, nil, errors.New("native one-off service secrets require explicit file projection")
	}
	entrypoint := service.Entrypoint
	if r.entrypoint != "" {
		entrypoint = []string{r.entrypoint}
	}
	if len(entrypoint) > 0 {
		encoded, err := json.Marshal(entrypoint)
		if err != nil {
			return nil, nil, nil, err
		}
		args = append(args, "--entrypoint", string(encoded))
	}
	args = append(args, service.Image)
	command := r.command
	if len(command) == 0 {
		command = service.Command
	}
	args = append(args, command...)
	return args, service.Environment, resources, nil
}

func (p PodmanProvider) runNativeComposeOneOff(ctx context.Context, project string, files []string, environment map[string]string, stdin io.Reader, stdout, stderr io.Writer, command []string) error {
	r, err := parseNativeOneOff(command)
	if err != nil {
		return err
	}
	model, path, err := quadletLoadComposeModel(files, "", environment)
	if err != nil {
		return err
	}
	selected, err := quadletSelectComposeServices(model, []string{r.service})
	if err != nil {
		return err
	}
	quadletEnsureDefaultNetwork(&model, selected)
	args, env, resources, err := nativeOneOffArgs(project, path, model, r)
	if err != nil {
		return err
	}
	for _, resource := range resources {
		exists, err := p.InspectProjectResource(ctx, project, resource)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("native one-off resource %s is not owned by project", resource.Name)
		}
	}
	// Put provider secrets only in a private env file, never native argv.
	if len(env) > 0 {
		file, err := os.CreateTemp("", ".baseharbor-oneoff-env-")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		var keys []string
		for key := range env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if strings.ContainsAny(key, "=\r\n\x00") || strings.ContainsAny(env[key], "\r\n\x00") {
				file.Close()
				return errors.New("invalid native one-off environment")
			}
			if _, err := fmt.Fprintf(file, "%s=%s\n", key, env[key]); err != nil {
				file.Close()
				return err
			}
		}
		if err := file.Close(); err != nil {
			return err
		}
		args = append([]string{args[0], "--env-file", file.Name()}, args[1:]...)
	}
	cmd := exec.CommandContext(ctx, p.CommandPath(), args...)
	cmd.Env = runtimeCommandEnv(p.CommandPath())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("native Podman one-off %s: %w", r.service, err)
	}
	return nil
}
