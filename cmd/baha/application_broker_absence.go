package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

// An existing deployment owner does not prove that a broker was materialized.
// Classify it as new only when protected owner files and native inventory both
// establish absence; any partial, foreign or unreadable state remains unknown.
func brokerOwnerUnmaterialized(ctx context.Context, provider bhruntime.RuntimeProvider, manifest application.Manifest, files application.RuntimeFiles) bool {
	// Podman Quadlet can retain user units without a container; container,
	// volume and network inventory alone cannot prove that owner absent.
	if provider.Kind() != bhruntime.ProviderDocker {
		return false
	}
	absolute, err := filepath.Abs(files.Dir)
	if err != nil {
		return false
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil || canonical != absolute {
		return false
	}
	info, err := os.Lstat(files.Dir)
	if err != nil || !protectedBrokerDirectory(info) {
		return false
	}
	bindings, err := os.Lstat(files.Bindings)
	if err == nil && !protectedBrokerDirectory(bindings) {
		return false
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false
	}
	for _, path := range []string{
		filepath.Join(files.Dir, "broker.compose.yaml"),
		filepath.Join(files.Dir, "broker-image"),
		filepath.Join(files.Dir, "broker-docs-port"),
		filepath.Join(files.Dir, "workload.runtime-identity.override.yaml"),
		filepath.Join(files.Bindings, "runtime-broker"),
		filepath.Join(files.Bindings, "runtime-identity"),
		filepath.Dir(application.RuntimeIdentityTokenPath(files)),
	} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	project := runtimebroker.ProjectNameForRuntime(manifest, files)
	owned, err := provider.ListOwnedProjectResources(ctx, project)
	if err != nil || len(owned) != 0 {
		return false
	}
	expected, err := runtimebroker.OwnedRuntimeResources(manifest, files)
	if err != nil {
		return false
	}
	actual, err := provider.InspectProjectResources(ctx, project, expected)
	return err == nil && len(actual) == 0
}

func protectedBrokerDirectory(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && info.IsDir() && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0077 == 0 && stat.Uid == uint32(os.Geteuid())
}
