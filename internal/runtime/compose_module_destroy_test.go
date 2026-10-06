package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsolidatedDestroyRemovesAnonymousVolumesAndPreservesExternalVolumes(t *testing.T) {
	for _, destroy := range []bool{false, true} {
		root := t.TempDir()
		logPath := filepath.Join(root, "calls")
		t.Setenv("CALL_LOG", logPath)
		command := filepath.Join(root, "runtime")
		script := `#!/bin/sh
printf '%s\n' "$*" >> "$CALL_LOG"
for argument in "$@"; do
  if [ "$argument" = config ]; then
    printf '%s\n' '{"services":{"provider":{}},"volumes":{"owned":{"name":"owned-data"},"foreign":{"name":"external-data","external":true}},"networks":{}}'
    exit 0
  fi
done
exit 0
`
		if err := os.WriteFile(command, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		backend := NewCLIBackend(command)
		var err error
		if destroy {
			err = backend.DestroyProject(context.Background(), "bh-test-shared", "compose.yaml", "runtime.env")
		} else {
			err = backend.DownProject(context.Background(), "bh-test-shared", "compose.yaml", "runtime.env")
		}
		if err != nil {
			t.Fatal(err)
		}
		calls, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatal(err)
		}
		output := string(calls)
		if strings.Contains(output, "volume rm -f external-data") {
			t.Fatal("external data volume deleted")
		}
		if destroy {
			if !strings.Contains(output, "rm -f -s -v provider") || !strings.Contains(output, "volume rm -f owned-data") {
				t.Fatalf("destroy omitted owned data cleanup: %s", output)
			}
		} else if !strings.Contains(output, "rm -f -s provider") || strings.Contains(output, " -v ") || strings.Contains(output, "volume rm") {
			t.Fatalf("down deleted persisted data: %s", output)
		}
	}
}
