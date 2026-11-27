package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// repositoryRecoveryBackupRuntime is a fake compose RuntimeProvider for the
// destroy-plan path. DestroyOwnedProjectResources records and removes the
// selected volumes; InspectProjectResources reports what survives, so a faithful
// full-destroy must delete every owned volume and verify none remains while a
// foreign volume is never selected for deletion.
type repositoryRecoveryBackupRuntime struct {
	bhruntime.RuntimeProvider
	deleted []string
	remain  map[string][]bhruntime.ProjectResource
}

func (r *repositoryRecoveryBackupRuntime) DestroyOwnedProjectResources(_ context.Context, _ string, selected []bhruntime.ProjectResource) error {
	for _, res := range selected {
		r.deleted = append(r.deleted, res.Name)
		delete(r.remain, res.Name)
	}
	return nil
}

func (r *repositoryRecoveryBackupRuntime) InspectProjectResources(_ context.Context, _ string, _ []bhruntime.ProjectResource) ([]bhruntime.ProjectResource, error) {
	remaining := make([]bhruntime.ProjectResource, 0, len(r.remain))
	for _, list := range r.remain {
		remaining = append(remaining, list...)
	}
	return remaining, nil
}

func newRepositoryRecoveryBackupRuntime() *repositoryRecoveryBackupRuntime {
	return &repositoryRecoveryBackupRuntime{remain: map[string][]bhruntime.ProjectResource{}}
}

// TestDestroyRepositoryDataVolumesRecoveryBackupAuthoritativeOwnership routes the
// owned recovery/backup volume and the foreign substring volume through the real
// destroy-plan rendering and deletion path (card 0428-329): the owned recovery/
// backup volume carries the authoritative io.baseharbor.backup label and is
// exclusively owned, so it is selected for deletion and verified removed; the
// foreign volume keeps an unrelated label substring and stays outside the
// destroy scope. No automatic consent or automatic deletion is added.
func TestDestroyRepositoryDataVolumesRecoveryBackupAuthoritativeOwnership(t *testing.T) {
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
			inventory := bhruntime.NewCLIBackend(command)
			project := "app-project"
			volumes, err := inventory.InventoryRepositoryVolumes(context.Background(), project)
			if err != nil {
				t.Fatal(err)
			}
			byName := map[string]bhruntime.RepositoryVolume{}
			for _, v := range volumes {
				byName[v.Name] = v
			}
			if len(volumes) != 3 {
				t.Fatalf("inventory: %+v", volumes)
			}

			runtime := newRepositoryRecoveryBackupRuntime()
			exec := &applicationDestroyExecution{
				manifest:          application.Manifest{Name: "app", Environment: "test"},
				files:             application.RuntimeFiles{Project: project},
				compose:           runtime,
				repositoryVolumes: volumes,
			}
			if err := exec.destroyRepositoryDataVolumes(context.Background()); err != nil {
				t.Fatalf("owned recovery/backup volume was not destroyed and verified: %v", err)
			}
			// Only the exclusively owned volumes (state + owned-backup) are deleted;
			// the foreign volume is never selected for a destructive command.
			if len(runtime.deleted) != 2 {
				t.Fatalf("expected two owned volumes deleted, got %d: %v", len(runtime.deleted), runtime.deleted)
			}
			deleted := map[string]bool{}
			for _, name := range runtime.deleted {
				deleted[name] = true
			}
			if !deleted["state"] || !deleted["owned-backup"] {
				t.Fatalf("owned plain and recovery/backup volume must be deleted: %v", runtime.deleted)
			}
			if deleted["foreign-backup"] {
				t.Fatalf("foreign volume must never be selected for deletion: %v", runtime.deleted)
			}

			var plan bytes.Buffer
			renderRepositoryDestroyVolumes(&plan, volumes, project)
			output := plan.String()
			if !strings.Contains(output, "delete data volume: owned-backup") {
				t.Fatalf("owned recovery/backup volume must render as a delete in the destroy plan: %s", output)
			}
			if strings.Contains(output, "preserve volume: owned-backup") {
				t.Fatalf("owned recovery/backup volume must not be preserved: %s", output)
			}
			if !strings.Contains(output, "preserve volume: foreign-backup") {
				t.Fatalf("foreign volume must render as preserved in the destroy plan: %s", output)
			}
		})
	}
}
