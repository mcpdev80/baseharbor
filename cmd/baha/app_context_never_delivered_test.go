package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// This only authorizes discovery of a never-delivered repository for local
// destroy. It must never turn an unowned deployment into a readable/writable
// deployment record, or admit a foreign owner.
func TestNeverDeliveredLocalDestroyProofIsOneShotAndFailClosed(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	target := deployment.ResolvedTarget{
		Name: "local", RuntimeProvider: "docker",
		AccessProvider: "local", AccessReference: "local", Scope: "default",
	}
	manifest := application.New("never-delivered", "dev", false, false, false)
	root, err := deployment.TargetStateRoot(target.Name)
	if err != nil {
		t.Fatal(err)
	}
	missing := deployment.ErrDeploymentOwnerMissing
	allowed := func(target deployment.ResolvedTarget, err error) bool {
		return eligibleNeverDeliveredLocalDestroy(target, root, manifest, "destroy", err)
	}
	if allowed(target, missing) {
		t.Fatal("missing proof admitted a Coreless destroy")
	}
	if err := deployment.CreateNeverDeliveredProof(target.Name, manifest.ApplicationID, manifest.Name, manifest.Environment); err != nil {
		t.Fatal(err)
	}
	if !allowed(target, missing) {
		t.Fatal("valid local never-delivered proof was not recognized")
	}
	if allowed(target, deployment.ErrDeploymentOwnerForeign) {
		t.Fatal("foreign installation was admitted")
	}
	if eligibleNeverDeliveredLocalDestroy(target, root, manifest, "apply", missing) {
		t.Fatal("proof authorized an apply mutation")
	}
	foreign := target
	foreign.AccessProvider = "remote"
	if allowed(foreign, missing) {
		t.Fatal("remote access was admitted as local")
	}
	foreign = target
	foreign.TenantID = "tenant"
	if allowed(foreign, missing) {
		t.Fatal("tenant was admitted as local")
	}
	deployments := filepath.Join(root, "deployments")
	if err := os.MkdirAll(deployments, 0o700); err != nil {
		t.Fatal(err)
	}
	if allowed(target, missing) {
		t.Fatal("existing deployment namespace was ignored")
	}
	if err := os.Remove(deployments); err != nil {
		t.Fatal(err)
	}
	id := deployment.DeploymentIdentity{
		Target: target.Name, ApplicationID: manifest.ApplicationID,
		Application: manifest.Name, Environment: manifest.Environment,
	}
	if err := deployment.ConsumeNeverDeliveredProof(id); err != nil {
		t.Fatal(err)
	}
	if allowed(target, missing) {
		t.Fatal("consumed proof was reused")
	}
	if !errors.Is(missing, deployment.ErrDeploymentOwnerMissing) {
		t.Fatal("incorrect owner error classification")
	}
}
