package containersecurity

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPodmanOwnershipUsesInspectedRunningState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command fixture")
	}
	root := t.TempDir()
	script := `#!/bin/sh
case "$*" in
  'container ls -aq') printf 'live\nstopped\n' ;;
  'container inspect live') printf '%s\n' '[{"Config":{"Labels":{"com.docker.compose.project":"owned","com.docker.compose.service":"sql"}},"State":{"Running":true}}]' ;;
  'container inspect stopped') printf '%s\n' '[{"Config":{"Labels":{"com.docker.compose.project":"owned","com.docker.compose.service":"sql"}},"State":{"Running":false}}]' ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(root, "podman"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BASEHARBOR_TEST_RUNTIME", "podman")
	id, err := composeServiceContainerID(context.Background(), "owned", "sql")
	if err != nil || id != "live" {
		t.Fatalf("live owned selection: id=%s err=%v", id, err)
	}
	if _, err := composeServiceContainerID(context.Background(), "foreign", "sql"); err == nil {
		t.Fatal("foreign ownership accepted")
	}
}
