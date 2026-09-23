package runtime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func quadletGeneratorPath() (string, error) {
	if path, err := exec.LookPath("podman-system-generator"); err == nil {
		return path, nil
	}
	for _, path := range []string{
		"/usr/lib/systemd/system-generators/podman-system-generator",
		"/usr/libexec/podman/quadlet",
	} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	return "", errors.New("Podman Quadlet generator not found")
}

// QuadletAvailable reports whether this host can run rootless Podman Quadlet units.
func QuadletAvailable(ctx context.Context) bool {
	if _, err := quadletGeneratorPath(); err != nil {
		return false
	}
	path, err := exec.LookPath("podman")
	if err != nil {
		return false
	}
	return exec.CommandContext(ctx, path, "version").Run() == nil
}

func quadletUserUnitDir() (string, error) {
	if config := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); config != "" {
		return filepath.Join(config, "containers", "systemd"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "containers", "systemd"), nil
}

func quadletUserRuntimeEnv() []string {
	env := os.Environ()
	uid := os.Getuid()
	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", uid)
		env = append(env, "XDG_RUNTIME_DIR="+runtimeDir)
	}
	if strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) == "" {
		env = append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(runtimeDir, "bus"))
	}
	return env
}

func quadletSystemctl(ctx context.Context, input []byte, args ...string) (string, error) {
	path, err := exec.LookPath("systemctl")
	if err != nil {
		return "", err
	}
	full := append([]string{"--user"}, args...)
	cmd := exec.CommandContext(ctx, path, full...)
	cmd.Env = quadletUserRuntimeEnv()
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
		return stdout.String(), fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}

func quadletValidateProject(ctx context.Context, project QuadletProject) error {
	dir, err := os.MkdirTemp("", "baseharbor-quadlet-validate-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := WriteQuadletProject(dir, project); err != nil {
		return err
	}
	generator, err := quadletGeneratorPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, generator, "--user", "--dryrun")
	cmd.Env = append(os.Environ(), "QUADLET_UNIT_DIRS="+dir)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("validate Quadlet project %s: %s", project.Project, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func quadletInstalledProjectFiles(dir, project string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	prefix := sanitizeQuadletName(project) + "-"
	var result []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		switch filepath.Ext(entry.Name()) {
		case ".container", ".build", ".network", ".volume", ".env":
			result = append(result, entry.Name())
		}
	}
	sort.Strings(result)
	return result, nil
}

func quadletUnitForFile(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	switch filepath.Ext(name) {
	case ".container":
		return base + ".service"
	case ".build":
		return base + "-build.service"
	case ".network":
		return base + "-network.service"
	case ".volume":
		return base + "-volume.service"
	default:
		return ""
	}
}

func quadletProjectServiceUnits(project QuadletProject, selected []string) ([]string, error) {
	if len(selected) == 0 {
		selected = make([]string, 0, len(project.ServiceUnits))
		for service := range project.ServiceUnits {
			selected = append(selected, service)
		}
	}
	sort.Strings(selected)
	result := make([]string, 0, len(selected))
	for _, service := range selected {
		unit, ok := project.ServiceUnits[service]
		if !ok {
			return nil, fmt.Errorf("Quadlet service %q is not part of project %s", service, project.Project)
		}
		result = append(result, unit)
	}
	return result, nil
}

func quadletRenderProject(composeFile, envFile, project string) (QuadletProject, error) {
	return RenderComposeProjectQuadlets(composeFile, envFile, project)
}

func quadletRenderProjectFiles(composeFiles []string, environment map[string]string, project string, selected []string) (QuadletProject, error) {
	return RenderComposeProjectFilesQuadletsEnv(composeFiles, "", environment, project, selected...)
}

func quadletInstallProject(ctx context.Context, project QuadletProject) error {
	if err := quadletValidateProject(ctx, project); err != nil {
		return err
	}
	dir, err := quadletUserUnitDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	existing, err := quadletInstalledProjectFiles(dir, project.Project)
	if err != nil {
		return err
	}
	desired := make(map[string]struct{}, len(project.Files))
	for name := range project.Files {
		desired[name] = struct{}{}
	}
	for _, name := range existing {
		if _, keep := desired[name]; keep {
			continue
		}
		if unit := quadletUnitForFile(name); unit != "" && strings.HasSuffix(name, ".container") {
			_, _ = quadletSystemctl(ctx, nil, "stop", unit)
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := WriteQuadletProject(dir, project); err != nil {
		return err
	}
	_, err = quadletSystemctl(ctx, nil, "daemon-reload")
	return err
}

func quadletStartProject(ctx context.Context, project QuadletProject, selected []string) error {
	if err := quadletInstallProject(ctx, project); err != nil {
		return err
	}
	units, err := quadletProjectServiceUnits(project, selected)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return nil
	}
	_, err = quadletSystemctl(ctx, nil, append([]string{"start"}, units...)...)
	return err
}

func quadletStopProject(ctx context.Context, project QuadletProject, selected []string) error {
	units, err := quadletProjectServiceUnits(project, selected)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return nil
	}
	_, err = quadletSystemctl(ctx, nil, append([]string{"stop"}, units...)...)
	return err
}

func quadletRemoveProject(ctx context.Context, project QuadletProject, destroyVolumes bool) error {
	_ = quadletStopProject(ctx, project, nil)
	dir, err := quadletUserUnitDir()
	if err != nil {
		return err
	}
	files, err := quadletInstalledProjectFiles(dir, project.Project)
	if err != nil {
		return err
	}

	var networkUnits, volumeUnits []string
	for _, name := range files {
		switch filepath.Ext(name) {
		case ".network":
			if unit := quadletUnitForFile(name); unit != "" {
				networkUnits = append(networkUnits, unit)
			}
		case ".volume":
			if destroyVolumes {
				if unit := quadletUnitForFile(name); unit != "" {
					volumeUnits = append(volumeUnits, unit)
				}
			}
		}
	}
	if len(networkUnits) > 0 {
		_, _ = quadletSystemctl(ctx, nil, append([]string{"stop"}, networkUnits...)...)
	}
	if len(volumeUnits) > 0 {
		_, _ = quadletSystemctl(ctx, nil, append([]string{"stop"}, volumeUnits...)...)
	}

	for _, name := range files {
		if !destroyVolumes && strings.HasSuffix(name, ".volume") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_, err = quadletSystemctl(ctx, nil, "daemon-reload")
	return err
}

func quadletExec(ctx context.Context, runtimeCommand, container string, input []byte, args ...string) (string, error) {
	full := []string{"exec"}
	if input != nil {
		full = append(full, "-i")
	}
	full = append(full, container)
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, runtimeCommand, full...)
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
		return stdout.String(), fmt.Errorf("podman exec %s: %s", container, message)
	}
	return stdout.String(), nil
}

func quadletLogs(ctx context.Context, runtimeCommand string, project QuadletProject, services []string) (string, error) {
	if len(services) == 0 {
		for service := range project.Containers {
			services = append(services, service)
		}
	}
	sort.Strings(services)
	var result strings.Builder
	for _, service := range services {
		container, ok := project.Containers[service]
		if !ok {
			return "", fmt.Errorf("Quadlet service %q is not part of project %s", service, project.Project)
		}
		cmd := exec.CommandContext(ctx, runtimeCommand, "logs", "--tail", "120", container)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return result.String(), fmt.Errorf("podman logs %s: %s", container, strings.TrimSpace(stderr.String()))
		}
		if result.Len() > 0 {
			result.WriteByte('\n')
		}
		fmt.Fprintf(&result, "==> %s <==\n%s", service, stdout.String())
	}
	return result.String(), nil
}

func quadletResolveComposeFiles(workdir string, composeFiles []string) ([]string, error) {
	result := make([]string, 0, len(composeFiles))
	for _, file := range composeFiles {
		file = strings.TrimSpace(file)
		if file == "" {
			return nil, errors.New("Compose file path is empty")
		}
		if !filepath.IsAbs(file) && strings.TrimSpace(workdir) != "" {
			file = filepath.Join(workdir, file)
		}
		absolute, err := filepath.Abs(file)
		if err != nil {
			return nil, err
		}
		result = append(result, absolute)
	}
	return result, nil
}
