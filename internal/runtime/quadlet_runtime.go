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
	dir, err := quadletUserUnitDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	unchanged, err := quadletProjectInstalledUnchanged(dir, project)
	if err != nil {
		return err
	}
	if unchanged {
		return nil
	}

	if err := quadletValidateProject(ctx, project); err != nil {
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

func quadletProjectInstalledUnchanged(dir string, project QuadletProject) (bool, error) {
	existing, err := quadletInstalledProjectFiles(dir, project.Project)
	if err != nil {
		return false, err
	}
	if len(existing) != len(project.Files) {
		return false, nil
	}

	for _, name := range existing {
		desired, ok := project.Files[name]
		if !ok {
			return false, nil
		}
		actual, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return false, err
		}
		if string(actual) != desired {
			return false, nil
		}
	}
	return true, nil
}

func quadletProjectBuildUnits(project QuadletProject, selected []string) []string {
	selectedSet := map[string]struct{}{}
	for _, service := range selected {
		selectedSet[service] = struct{}{}
	}
	var units []string
	for name := range project.Files {
		if filepath.Ext(name) != ".build" {
			continue
		}
		if len(selectedSet) > 0 {
			matched := false
			for service := range selectedSet {
				expected := project.Project + "-" + sanitizeQuadletName(service) + ".build"
				if name == expected {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if unit := quadletUnitForFile(name); unit != "" {
			units = append(units, unit)
		}
	}
	sort.Strings(units)
	return units
}

func quadletBuildProject(ctx context.Context, project QuadletProject, selected []string) error {
	if err := quadletInstallProject(ctx, project); err != nil {
		return err
	}
	units := quadletProjectBuildUnits(project, selected)
	if len(units) == 0 {
		return nil
	}
	_, err := quadletSystemctl(ctx, nil, append([]string{"restart"}, units...)...)
	return err
}

func quadletStartProject(ctx context.Context, project QuadletProject, selected []string) error {
	return quadletStartProjectMode(ctx, project, selected, true)
}

func quadletStartProjectNoBuild(ctx context.Context, project QuadletProject, selected []string) error {
	return quadletStartProjectMode(ctx, project, selected, false)
}

func quadletStartProjectMode(ctx context.Context, project QuadletProject, selected []string, build bool) error {
	if err := quadletInstallProject(ctx, project); err != nil {
		return err
	}

	networks, volumes, builds := quadletProjectResourceUnits(project)
	if err := quadletEnsureResourceUnits(ctx, project, networks, "network", "NetworkName"); err != nil {
		return err
	}
	if err := quadletEnsureResourceUnits(ctx, project, volumes, "volume", "VolumeName"); err != nil {
		return err
	}
	if build && len(builds) > 0 {
		if _, err := quadletSystemctl(ctx, nil, append([]string{"start"}, builds...)...); err != nil {
			return err
		}
	}

	units, err := quadletProjectServiceUnits(project, selected)
	if err != nil {
		return err
	}
	if len(units) == 0 {
		return nil
	}
	if _, err := quadletSystemctl(ctx, nil, append([]string{"start"}, units...)...); err != nil {
		return err
	}
	return quadletEnsureServiceUnitsActive(ctx, units)
}

func quadletEnsureServiceUnitsActive(ctx context.Context, units []string) error {
	for _, unit := range units {
		if _, err := quadletSystemctl(ctx, nil, "is-active", "--quiet", unit); err == nil {
			continue
		}
		status, statusErr := quadletSystemctlCombined(ctx, "status", "--no-pager", "--full", unit)
		if statusErr != nil && strings.TrimSpace(status) == "" {
			status = statusErr.Error()
		}
		return fmt.Errorf("Quadlet service unit %s did not remain active: %s", unit, strings.TrimSpace(status))
	}
	return nil
}

func quadletSystemctlCombined(ctx context.Context, args ...string) (string, error) {
	path, err := exec.LookPath("systemctl")
	if err != nil {
		return "", err
	}
	full := append([]string{"--user"}, args...)
	cmd := exec.CommandContext(ctx, path, full...)
	cmd.Env = quadletUserRuntimeEnv()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("systemctl --user %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

func quadletEnsureResourceUnits(ctx context.Context, project QuadletProject, units []string, kind, directive string) error {
	if len(units) == 0 {
		return nil
	}
	for _, unit := range units {
		file := quadletFileForUnit(project, unit, kind)
		if file == "" {
			return fmt.Errorf("Quadlet %s unit %s has no matching project file", kind, unit)
		}
		resource := quadletDirectiveValue(project.Files[file], directive)
		if resource == "" {
			return fmt.Errorf("Quadlet %s file %s has no %s directive", kind, file, directive)
		}
		exists, err := quadletRuntimeResourceExists(ctx, kind, resource)
		if err != nil {
			return err
		}
		if exists {
			if _, err := quadletSystemctl(ctx, nil, "start", unit); err != nil {
				return err
			}
			continue
		}
		_, _ = quadletSystemctl(ctx, nil, "reset-failed", unit)
		if _, err := quadletSystemctl(ctx, nil, "restart", unit); err != nil {
			return err
		}
		exists, err = quadletRuntimeResourceExists(ctx, kind, resource)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("Quadlet %s resource %s is missing after restarting %s", kind, resource, unit)
		}
	}
	return nil
}

func quadletFileForUnit(project QuadletProject, unit, kind string) string {
	var suffix string
	switch kind {
	case "network":
		suffix = ".network"
	case "volume":
		suffix = ".volume"
	default:
		return ""
	}
	for name := range project.Files {
		if filepath.Ext(name) != suffix {
			continue
		}
		if quadletUnitForFile(name) == unit {
			return name
		}
	}
	return ""
}

func quadletRuntimeResourceExists(ctx context.Context, kind, name string) (bool, error) {
	path, err := exec.LookPath("podman")
	if err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, path, kind, "exists", name)
	if err := cmd.Run(); err == nil {
		return true, nil
	} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	} else {
		return false, fmt.Errorf("check Podman %s resource %s: %w", kind, name, err)
	}
}

func quadletProjectResourceUnits(project QuadletProject) (networks, volumes, builds []string) {
	for name := range project.Files {
		unit := quadletUnitForFile(name)
		if unit == "" {
			continue
		}
		switch filepath.Ext(name) {
		case ".network":
			networks = append(networks, unit)
		case ".volume":
			volumes = append(volumes, unit)
		case ".build":
			builds = append(builds, unit)
		}
	}
	sort.Strings(networks)
	sort.Strings(volumes)
	sort.Strings(builds)
	return networks, volumes, builds
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
	var networkNames, volumeNames []string
	for _, name := range files {
		content := project.Files[name]
		switch filepath.Ext(name) {
		case ".network":
			if unit := quadletUnitForFile(name); unit != "" {
				networkUnits = append(networkUnits, unit)
			}
			if resource := quadletDirectiveValue(content, "NetworkName"); resource != "" {
				networkNames = append(networkNames, resource)
			}
		case ".volume":
			if destroyVolumes {
				if unit := quadletUnitForFile(name); unit != "" {
					volumeUnits = append(volumeUnits, unit)
				}
				if resource := quadletDirectiveValue(content, "VolumeName"); resource != "" {
					volumeNames = append(volumeNames, resource)
				}
			}
		}
	}
	if len(networkUnits) > 0 {
		_, _ = quadletSystemctl(ctx, nil, append([]string{"stop"}, networkUnits...)...)
	}
	if err := quadletRemoveRuntimeResources(ctx, "network", networkNames); err != nil {
		return err
	}
	if len(volumeUnits) > 0 {
		_, _ = quadletSystemctl(ctx, nil, append([]string{"stop"}, volumeUnits...)...)
	}
	if destroyVolumes {
		if err := quadletRemoveRuntimeResources(ctx, "volume", volumeNames); err != nil {
			return err
		}
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

func quadletDirectiveValue(content, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func quadletRemoveRuntimeResources(ctx context.Context, kind string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	path, err := exec.LookPath("podman")
	if err != nil {
		return err
	}
	sort.Strings(names)
	args := []string{kind, "rm", "-f"}
	args = append(args, names...)
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("remove Podman %s resources: %s", kind, message)
	}
	return nil
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
