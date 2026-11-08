package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedNetworkPoolExhaustionReportsSafeRecovery(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stderr   string
		recovery bool
	}{
		{
			name: "docker-pool-exhausted",
			stderr: "all predefined address pools have been fully subnetted",
			recovery: true,
		},
		{
			name: "docker-overlap",
			stderr: "could not find an available, non-overlapping IPv4 address pool among the defaults to assign to the network",
			recovery: true,
		},
		{
			name: "unrelated-permission-denied",
			stderr: "permission denied",
			recovery: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fake-docker")
			script := "#!/bin/sh\nif [ \"$1\" = network ] && [ \"$2\" = ls ]; then exit 0; fi\nprintf '%s\\n' " + "'" + tc.stderr + "'" + " >&2\nexit 1\n"
			if err := os.WriteFile(path, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			err := NewCLIBackend(path).EnsureManagedNetwork(context.Background(), "bh-test-target-network")
			if err == nil {
				t.Fatal("fake network create error was swallowed")
			}
			message := err.Error()
			if !strings.Contains(message, tc.stderr) {
				t.Fatalf("runtime error not preserved: %s", message)
			}
			if tc.recovery {
				for _, want := range []string{"bh-test-target-network", "network ls", "docker network prune", "automatic cleanup is not performed"} {
					if !strings.Contains(message, want) {
						t.Fatalf("missing recovery guidance %q: %s", want, message)
					}
				}
			} else if strings.Contains(message, "network prune") {
				t.Fatalf("unrelated error incorrectly classified as pool exhaustion: %s", message)
			}
		})
	}
}
