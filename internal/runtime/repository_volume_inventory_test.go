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

// TestRepositoryVolumeInventoryRecoveryBackupFullDestroy proves the full-destroy
// contract: an owned recovery/backup volume carries an authoritative
// io.baseharbor.backup label and is exclusively owned (not preserved by a
// recovery/backup substring), while a foreign volume keeps only an unrelated
// label substring and stays outside the destroy scope. Acceptance for card
// 0428-329 states full destroy deletes every installation-owned volume
// including recovery/backup volumes with no label-based preservation
// exception, and that ownership is authoritative, not a substring heuristic.
func TestRepositoryVolumeInventoryRecoveryBackupFullDestroy(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		t.Run(engine, func(t *testing.T) {
			command := filepath.Join(t.TempDir(), engine)
			script := `#!/bin/sh
case "$1 $2" in
"container ls") printf 'app\n' ;;
"container inspect") printf '%s\n' '/app|app-project||[{"Type":"volume","Name":"state"},{"Type":"volume","Name":"owned-backup"},{"Type":"volume","Name":"foreign-backup"}]' ;;
"volume ls") printf 'state\nowned-backup\nforeign-backup\n' ;;
"volume inspect")
case "$5" in
state) printf '{"Name":"state","Labels":{"com.docker.compose.project":"app-project"}}' ;;
owned-backup) printf '{"Name":"owned-backup","Labels":{"com.docker.compose.project":"app-project","io.baseharbor.backup":"true"}}' ;;
foreign-backup) printf '{"Name":"foreign-backup","Labels":{"com.docker.compose.project":"other-project","io.baseharbor.foo":"backup-substr"}}' ;;
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
			byName := map[string]RepositoryVolume{}
			for _, v := range volumes {
				byName[v.Name] = v
			}
			if len(volumes) != 3 {
				t.Fatalf("inventory: %+v", volumes)
			}
			// Owned recovery/backup volume: authoritative ownership, NOT preserved by label.
			if owned, ok := byName["owned-backup"]; !ok || owned.Project != "app-project" || owned.Shared {
				t.Fatalf("owned recovery/backup volume must be exclusively owned (Shared=false) for full destroy: %+v", owned)
			}
			// Owned plain volume: exclusively owned.
			if state, ok := byName["state"]; !ok || state.Project != "app-project" || state.Shared {
				t.Fatalf("owned plain volume must be exclusively owned: %+v", state)
			}
			// Foreign volume: ownership stays foreign (Shared), never adopted into the destroy scope.
			if foreign, ok := byName["foreign-backup"]; !ok || foreign.Project != "other-project" || !foreign.Shared {
				t.Fatalf("foreign volume with unrelated backup substring must stay foreign/Shared: %+v", foreign)
			}
		})
	}
}
