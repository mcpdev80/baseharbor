package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func configureTestTarget(t *testing.T) deployment.ResolvedTarget {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	cfg := deployment.Config{
		Version:       deployment.ConfigVersion,
		DefaultTarget: "docker-dev",
		Access: map[string]deployment.AccessDefinition{
			"local-docker": {Provider: "docker", Reference: "local"},
		},
		Targets: map[string]deployment.TargetDefinition{
			"docker-dev": {
				Runtime: deployment.RuntimeDefinition{Provider: "docker"},
				Access:  deployment.TargetAccess{Reference: "local-docker"},
				Scope:   "local",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	target, err := cfg.ResolveTarget("", "")
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func registerTestDeployment(t *testing.T, target deployment.ResolvedTarget, m application.Manifest, repositoryRoot, manifestPath string) deployment.DeploymentRecord {
	t.Helper()
	intent, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	record := deployment.DeploymentRecord{
		Version: deployment.DeploymentRecordVersion,
		Identity: deployment.DeploymentIdentity{
			Target:      target.Name,
			Application: m.Name,
			Environment: m.Environment,
		},
		Source: deployment.DeploymentSource{
			Kind:       "repository",
			Repository: repositoryRoot,
			Manifest:   manifestPath,
		},
		Applied: deployment.AppliedDeployment{
			Intent:          intent,
			RuntimeProvider: target.RuntimeProvider,
		},
	}
	if err := deployment.SaveDeploymentRecord(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func testDeploymentStore(t *testing.T, target deployment.ResolvedTarget, m application.Manifest) application.Store {
	t.Helper()
	root, err := deployment.DeploymentRoot(deployment.DeploymentIdentity{
		Target:      target.Name,
		Application: m.Name,
		Environment: m.Environment,
	})
	if err != nil {
		t.Fatal(err)
	}
	return application.Store{Root: filepath.Join(root, "state"), Namespace: target.Name}
}
