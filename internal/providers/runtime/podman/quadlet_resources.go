package podman

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

func quadletRemoveRuntimeResources(ctx context.Context, kind string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	path, err := exec.LookPath("podman")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		args := quadletRemoveRuntimeResourceArgs(kind, name)
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Env = runtimeCommandEnv(path)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			if quadletRuntimeResourceMissing(kind, message) {
				continue
			}
			if kind == "network" && quadletNetworkResourceInUse(message) {
				continue
			}
			return fmt.Errorf("remove Podman %s resource %s: %s", kind, name, message)
		}
	}
	return nil
}

func quadletRemoveRuntimeResourceArgs(kind, name string) []string {
	args := []string{kind, "rm"}
	if kind == "container" || kind == "volume" {
		args = append(args, "-f")
	}
	return append(args, name)
}

func quadletNetworkResourceInUse(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(lower, "network is being used") ||
		strings.Contains(lower, "has associated containers") ||
		strings.Contains(lower, "active endpoints")
}

func quadletRuntimeResourceMissing(kind, message string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	lower := strings.ToLower(strings.TrimSpace(message))
	if kind == "" || lower == "" {
		return false
	}
	return strings.Contains(lower, kind+" not found") ||
		strings.Contains(lower, "unable to find "+kind) ||
		strings.Contains(lower, "no such "+kind)
}
