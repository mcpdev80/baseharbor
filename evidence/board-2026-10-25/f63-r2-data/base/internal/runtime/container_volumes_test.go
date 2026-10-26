package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMountedVolumeInventoryUsesEngineAnonymityAndAllConsumers(t *testing.T) {
	command := filepath.Join(t.TempDir(), "runtime")
	script := `#!/bin/sh
case "$1 $2" in
"container ls") printf 'owned\nforeign\n' ;;
"container inspect")
printf '%s\n' '/owned|bh-test-shared||[{"Type":"volume","Name":"exclusive"},{"Type":"volume","Name":"shared"},{"Type":"volume","Name":"external"},{"Type":"volume","Name":"podman-anon"},{"Type":"bind","Name":""}]' '/foreign|another-project||[{"Type":"volume","Name":"shared"}]' ;;
"volume inspect")
case "$5" in
exclusive|shared) printf '{"Name":"%s","Labels":{"com.docker.volume.anonymous":""}}' "$5" ;;
external) printf '{"Name":"external","Labels":{}}' ;;
podman-anon) printf '{"Name":"podman-anon","Anonymous":true}' ;;
esac ;;
*) exit 9 ;;
esac
`
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	volumes, err := NewCLIBackend(command).InventoryOwnedContainerVolumes(context.Background(), "bh-test-shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(volumes) != 4 {
		t.Fatalf("inventory: %#v", volumes)
	}
	want := map[string]bool{"exclusive": true, "podman-anon": true, "external": false, "shared": false}
	for _, volume := range volumes {
		if volume.Removable != want[volume.Name] || volume.Reason == "" {
			t.Fatalf("unsafe volume decision: %#v", volume)
		}
	}
}
