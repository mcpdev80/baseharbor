package podman

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ObserveProjectServiceCompletion reuses Quadlet's existing successful-exit
// authority when Podman has removed the one-shot container. It never starts or
// repairs a unit, synthesizes a container, or changes stored completion state.
func (p PodmanProvider) ObserveProjectServiceCompletion(ctx context.Context, project, compose, env, service string) (bool, error) {
	q, err := quadletRenderProject(compose, env, project)
	if err != nil {
		return false, err
	}
	unit, exists := q.ServiceUnits[service]
	if !exists || !q.CompletedServices[service] {
		return false, nil
	}
	dir, err := quadletUserUnitDir()
	if err != nil {
		return false, err
	}
	if !quadletCompletionFilesOwned(dir, q) {
		return false, nil
	}
	unchanged, err := quadletProjectInstalledUnchanged(dir, q)
	if err != nil || !unchanged {
		return false, err
	}
	properties, err := quadletSystemctl(ctx, nil, "show", unit, "--property=SourcePath,FragmentPath,DropInPaths,NeedDaemonReload", "--no-pager")
	if err != nil {
		return false, err
	}
	runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtimeDir == "" {
		// The existing Quadlet runtime environment owns this default as well.
		for _, value := range quadletUserRuntimeEnv() {
			if strings.HasPrefix(value, "XDG_RUNTIME_DIR=") {
				runtimeDir = strings.TrimPrefix(value, "XDG_RUNTIME_DIR=")
			}
		}
	}
	if !quadletCompletionUnitOwned(properties, dir, runtimeDir, unit) {
		return false, nil
	}
	// Reuse the exact existing completion predicate; bound its wait so this
	// observer stays read-only and an incomplete task remains unverified.
	probe, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	command := exec.CommandContext(probe, "/bin/sh", "-ec", quadletCompletedDependencyScript(unit))
	command.Env = quadletUserRuntimeEnv()
	return command.Run() == nil, nil
}

func quadletCompletionFilesOwned(dir string, project QuadletProject) bool {
	paths := []string{dir}
	for name := range project.Files {
		paths = append(paths, filepath.Join(dir, name))
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
			return false
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Getuid()) || path != dir && (!info.Mode().IsRegular() || stat.Nlink != 1) || path == dir && !info.IsDir() {
			return false
		}
	}
	return true
}

func quadletCompletionUnitOwned(raw, dir, runtimeDir, unit string) bool {
	if !filepath.IsAbs(dir) || !filepath.IsAbs(runtimeDir) {
		return false
	}
	properties := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return false
		}
		if _, duplicate := properties[key]; duplicate {
			return false
		}
		properties[key] = value
	}
	return len(properties) == 4 && properties["SourcePath"] == filepath.Join(dir, strings.TrimSuffix(unit, ".service")+".container") &&
		properties["FragmentPath"] == filepath.Join(runtimeDir, "systemd", "generator", unit) &&
		properties["NeedDaemonReload"] == "no" && properties["DropInPaths"] == ""
}
