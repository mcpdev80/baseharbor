package runtime

import (
	"os"
	"path/filepath"
	"strings"
)

func runtimeCommandEnv(command string) []string {
	env := os.Environ()
	if filepath.Base(strings.TrimSpace(command)) != "podman" {
		return env
	}
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, "XDG_CONFIG_HOME=") || strings.HasPrefix(entry, "XDG_DATA_HOME=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
