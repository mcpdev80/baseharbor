package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryVolumeInventoryOwnershipAndOrphans(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		t.Run(engine, func(t *testing.T) {
			command := filepath.Join(t.TempDir(), engine)
			script := `#!/bin/sh
case "$1 $2" in
"container ls") printf 'app\nforeign\n' ;;
"container inspect") printf '%s\n' '/app|app-project||[{"Type":"volume","Name":"external"},{"Type":"volume","Name":"shared"},{"Type":"volume","Name":"conflict"}]' '/foreign|other-project||[{"Type":"volume","Name":"shared"}]' ;;
"volume ls") printf 'orphan\nexternal\nshared\nconflict\nforeign\nrecovery\n' ;;
"volume inspect")
case "$5" in
orphan) printf '{"Name":"orphan","Labels":{"com.docker.compose.project":"app-project","com.docker.compose.volume":"state"}}' ;;
external) printf '{"Name":"external","Labels":{}}' ;;
shared) printf '{"Name":"shared","Labels":{"io.podman.compose.project":"app-project"}}' ;;
conflict) printf '{"Name":"conflict","Labels":{"com.docker.compose.project":"app-project","io.podman.compose.project":"other-project"}}' ;;
foreign) printf '{"Name":"foreign","Labels":{"com.docker.compose.project":"other-project"}}' ;;
recovery) printf '{"Name":"recovery","Labels":{"com.docker.compose.project":"app-project","io.baseharbor.recovery":"true"}}' ;;
esac ;;
*) exit 9 ;;
esac
`
			if err := os.WriteFile(command, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			volumes, err := NewCLIBackend(command).InventoryRepositoryVolumes(context.Background(), "app-project")
			if err != nil {
				t.Fatal(err)
			}
			if len(volumes) != 5 {
				t.Fatalf("inventory: %+v", volumes)
			}
			for _, volume := range volumes {
				if volume.Name == "foreign" || volume.Reason == "" || volume.Owner != "not recorded" {
					t.Fatalf("unsafe provenance %+v", volume)
				}
				if (volume.Name == "shared" || volume.Name == "conflict" || volume.Name == "recovery") && !volume.Shared {
					t.Fatalf("missing protection %+v", volume)
				}
				if volume.Name == "external" && volume.Project != "" {
					t.Fatalf("claimed external ownership %+v", volume)
				}
			}
		})
	}
}
