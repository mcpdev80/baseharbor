package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeRecoveryUsesSamePodmanStorageAsOwnershipInspection(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "podman")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "foreign-config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "foreign-data"))
	const script = `#!/bin/sh
if [ -n "$XDG_CONFIG_HOME" ] || [ -n "$XDG_DATA_HOME" ]; then
 echo foreign-storage-context >&2
 exit 42
fi
case "$1 $2" in
 "volume ls") echo owned_pg ;;
 "volume inspect") echo 'owned_pg|owned|owned' ;;
 "run --rm") cat >/dev/null ;;
esac
`
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	provider := Compose{command: command}
	if _, err := provider.directBinary(context.Background(), nil, "volume", "create", "owned_pg"); err != nil {
		t.Fatal(err)
	}
	if err := provider.SeedOwnedVolume(context.Background(), "owned", "owned_pg", "spilo@sha256:"+strings.Repeat("a", 64), "data", "101:103", strings.NewReader("verified-stream")); err != nil {
		t.Fatal(err)
	}
}
