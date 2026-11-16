package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func TestRepositoryReinitializationReusesActualIdentityOwners(t *testing.T) {
	for _, mode := range []string{"deterministic", "guided", "quick", "quick-json", "conflicting-owners"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			target := configureTestTarget(t)
			root := filepath.Join(t.TempDir(), "demo")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			t.Chdir(root)
			if err := os.WriteFile("compose.yaml", []byte("services:\n  api:\n    image: example.invalid/app:fixed\n"), 0600); err != nil {
				t.Fatal(err)
			}
			owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
			prior := application.WithWorkloadComponents(application.New("demo", "dev", false, false, false), "api")
			ownerPath, err := owner.Create(prior)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(ownerPath)
			if err != nil {
				t.Fatal(err)
			}
			registered := prior
			if mode == "conflicting-owners" {
				registered.ApplicationID = application.MustNewApplicationID()
			}
			id, err := deployment.NewDeploymentIdentity(target.Name, registered.ApplicationID, prior.Name, prior.Environment)
			if err != nil {
				t.Fatal(err)
			}
			intent, _ := json.Marshal(registered)
			record := deployment.DeploymentRecord{Identity: id, Source: deployment.DeploymentSource{Repository: root}, Applied: deployment.AppliedDeployment{Intent: intent, RuntimeProvider: target.RuntimeProvider}}
			if err := deployment.SaveDeploymentRecord(record); err != nil {
				t.Fatal(err)
			}
			beforeRecord, found, err := deployment.FindDeployment(target.Name, registered.ApplicationID, "dev")
			if err != nil || !found {
				t.Fatal("fixture deployment unavailable", err)
			}
			if mode == "deterministic" || mode == "conflicting-owners" {
				err = appInitCommand().Run(context.Background(), []string{"demo", "--sql", "--workload-component", "api", "--workload-source", "compose:compose.yaml"}, &bytes.Buffer{}, &bytes.Buffer{})
			} else if strings.HasPrefix(mode, "quick") {
				args := []string{"app", "init", "--quick", "--no-input"}
				if mode == "quick-json" {
					args = append(args, "--json")
				}
				err = runWithIO(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{})
			} else {
				proposal := application.WithWorkloadComponents(detectedApplicationManifest("demo", "dev", true, false, false, false, true), "api")
				err = writeRepositoryManifest(context.Background(), proposal, &bytes.Buffer{})
			}
			if mode == "conflicting-owners" {
				if err == nil || !strings.Contains(err.Error(), "conflicting application IDs") {
					t.Fatalf("genuine owner conflict accepted: %v", err)
				}
				if _, statErr := os.Stat(application.RepositoryManifestName); !os.IsNotExist(statErr) {
					t.Fatal("conflicting owners wrote source")
				}
			} else {
				if err != nil {
					t.Fatalf("init manufactured a conflicting proposal identity: %v", err)
				}
				authored, err := application.LoadManifestFile(application.RepositoryManifestName)
				if err != nil || authored.ApplicationID != "" {
					t.Fatal("canonical identity leaked into authored source", err)
				}
			}
			after, _ := os.ReadFile(ownerPath)
			canonical, _, ownerErr := owner.Load(prior.Name)
			current, found, loadErr := deployment.FindDeployment(target.Name, registered.ApplicationID, "dev")
			if ownerErr != nil || canonical.ApplicationID != prior.ApplicationID || mode == "conflicting-owners" && !bytes.Equal(before, after) || loadErr != nil || !found || !reflect.DeepEqual(current, beforeRecord) {
				t.Fatal("reinitialization changed canonical identity or retained deployment")
			}
		})
	}
}
