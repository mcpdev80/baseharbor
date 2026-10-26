package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestRestoreReconcilesPortableSourceWithValidatedArchiveIdentity(t *testing.T) {
	for _, mode := range []string{"selected", "registered", "fresh", "foreign-owner", "changed-intent"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			t.Setenv(application.ProviderScopeEnv(capability.ProviderPostgreSQL), "")
			target := configureTestTarget(t)
			root := t.TempDir()
			m := application.New("restore-source", "dev", true, false, false)
			authored := m
			if mode == "changed-intent" {
				authored.Services.Cache = true
			}
			path := filepath.Join(root, application.RepositoryManifestName)
			if err := os.WriteFile(path, []byte(authored.AuthoredYAML()), 0600); err != nil {
				t.Fatal(err)
			}
			owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
			var ownerPath string
			var beforeOwner []byte
			if mode != "fresh" {
				canonical := m
				if mode == "foreign-owner" {
					canonical.ApplicationID = application.MustNewApplicationID()
				}
				var err error
				ownerPath, err = owner.Create(canonical)
				if err != nil {
					t.Fatal(err)
				}
				beforeOwner, _ = os.ReadFile(ownerPath)
				id, err := deployment.NewDeploymentIdentity(target.Name, m.ApplicationID, m.Name, m.Environment)
				if err != nil {
					t.Fatal(err)
				}
				intent, _ := json.Marshal(m)
				placements, _ := json.Marshal(map[capability.ProviderKind]capability.ProviderPlacement{capability.ProviderPostgreSQL: {Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor}})
				if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{Identity: id, Source: deployment.DeploymentSource{Kind: "repository", Repository: root, Manifest: path}, Applied: deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider, GeneratedState: map[string]string{"backend_placements": string(placements)}}}); err != nil {
					t.Fatal(err)
				}
			}
			beforeSource, _ := os.ReadFile(path)
			cwd := root
			if mode == "registered" {
				cwd = t.TempDir()
			}
			t.Chdir(cwd)
			resolved, err := resolveRestoreTarget(context.Background(), application.Store{}, m)
			if mode == "foreign-owner" || mode == "changed-intent" {
				if err == nil {
					t.Fatal("foreign identity or changed desired state accepted")
				}
			} else if err != nil || resolved.Manifest.ApplicationID != m.ApplicationID || resolved.RepositoryRoot != root || !resolved.FromRepository {
				t.Fatalf("matching portable source rejected or replaced archive identity: %v", err)
			}
			if err == nil && mode != "fresh" {
				if err := revalidateApplicationProviderIntent(context.Background(), resolved); err != nil {
					t.Fatalf("unchanged retained restore owner falsely stale: %v", err)
				}
				placement, placementErr := application.ResolveProviderPlacement(resolved.Manifest, capability.ProviderPostgreSQL)
				if placementErr != nil || placement.Scope != capability.ScopeApplication {
					t.Fatalf("restore lost retained application placement: %v", placementErr)
				}
				record, loadErr := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				changed, _ := json.Marshal(map[capability.ProviderKind]capability.ProviderPlacement{capability.ProviderPostgreSQL: {Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}})
				record.Applied.GeneratedState["backend_placements"] = string(changed)
				if saveErr := deployment.SaveDeploymentRecord(record); saveErr != nil {
					t.Fatal(saveErr)
				}
				if driftErr := revalidateApplicationProviderIntent(context.Background(), resolved); driftErr == nil {
					t.Fatal("restore accepted changed retained provider placement")
				}
			}
			afterSource, _ := os.ReadFile(path)
			if !bytes.Equal(beforeSource, afterSource) {
				t.Fatal("restore preflight changed authored source")
			}
			if mode == "fresh" {
				if _, err := os.Stat(owner.Root); !os.IsNotExist(err) {
					t.Fatal("read-only restore preflight created a canonical owner")
				}
			} else {
				afterOwner, _ := os.ReadFile(ownerPath)
				if !bytes.Equal(beforeOwner, afterOwner) {
					t.Fatal("restore preflight replaced existing identity owner")
				}
			}
		})
	}
}
