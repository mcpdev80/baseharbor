package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestPrepareStagedCoreGitCandidatePinsCanonicalApplication(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init")
	git("remote", "add", "origin", "https://git.example/demo.git")
	manifest := application.New("demo", "dev", false, false, false)
	path := filepath.Join(root, application.RepositoryManifestName)
	if err := os.WriteFile(path, []byte(manifest.AuthoredYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", application.RepositoryManifestName)
	git("-c", "user.name=Test", "-c", "user.email=test@example.org", "commit", "-m", "immutable app")
	revision := git("rev-parse", "HEAD")
	identity, err := deployment.NewDeploymentIdentity("remote-docker", manifest.ApplicationID, "demo", "dev")
	if err != nil {
		t.Fatal(err)
	}
	source := development.SourceDefinition{
		ID: "app", Type: development.SourceRepository,
		Repository: "https://git.example/demo.git", Ref: revision,
	}
	recorded := deployment.DeploymentSource{Kind: "repository", Repository: root}
	candidate, err := prepareStagedCoreGitCandidate(context.Background(), source, recorded, identity, manifest)
	if err != nil {
		t.Fatalf("stage pinned Core candidate: %v", err)
	}
	defer candidate.Close()
	if len(candidate.Fingerprint) != 64 ||
		candidate.Submission.ApplicationID != identity.ApplicationID ||
		candidate.Submission.TargetName != identity.Target ||
		candidate.Submission.SourceRevision != revision {
		t.Fatalf("candidate identity mismatch: %+v", candidate)
	}
	originalFingerprint := candidate.Fingerprint
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if hash, err := fingerprintStagedCandidate(candidate.Root); err != nil || hash != originalFingerprint {
		t.Fatalf("pinned candidate changed with mutable working directory: hash=%s err=%v", hash, err)
	}
	if _, err := prepareStagedCoreGitCandidate(context.Background(), source, recorded, identity, manifest); err == nil {
		t.Fatal("dirty registered Core checkout was accepted for new candidate preparation")
	}
	if err := candidate.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(candidate.Root); !os.IsNotExist(err) {
		t.Fatalf("closed candidate still exists: %v", err)
	}
}

func TestPrepareStagedCoreGitCandidateRejectsForeignManifest(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init")
	git("remote", "add", "origin", "https://git.example/demo.git")
	foreign := application.New("other-app", "dev", false, false, false)
	if err := os.WriteFile(filepath.Join(root, application.RepositoryManifestName), []byte(foreign.AuthoredYAML()), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", application.RepositoryManifestName)
	git("-c", "user.name=Test", "-c", "user.email=test@example.org", "commit", "-m", "foreign app")
	manifest := application.New("demo", "dev", false, false, false)
	identity, err := deployment.NewDeploymentIdentity("remote-docker", manifest.ApplicationID, "demo", "dev")
	if err != nil {
		t.Fatal(err)
	}
	source := development.SourceDefinition{
		ID: "app", Type: development.SourceRepository,
		Repository: "https://git.example/demo.git", Ref: git("rev-parse", "HEAD"),
	}
	_, err = prepareStagedCoreGitCandidate(context.Background(), source,
		deployment.DeploymentSource{Kind: "repository", Repository: root}, identity, manifest)
	if err == nil {
		t.Fatal("foreign application manifest accepted as a Core-owned candidate")
	}
}

func TestFingerprintStagedCandidateIncludesExecutableMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "entrypoint.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho ready\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := fingerprintStagedCandidate(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	after, err := fingerprintStagedCandidate(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("candidate fingerprint did not bind the Git executable bit")
	}
}

func TestStagedCoreGitCandidateConsentPinsExactArtifact(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.yaml")
	if err := os.WriteFile(path, []byte("version: one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := fingerprintStagedCandidate(root)
	if err != nil {
		t.Fatal(err)
	}
	candidate := stagedCoreGitCandidate{
		Root: root, Fingerprint: fingerprint,
		Submission: delivery.CandidateSubmission{
			ContractVersion: delivery.CandidateContractVersion,
			ApplicationID: "b13e96c1-83a0-4dd2-90e5-54a2d2f3ee8a",
			TargetName: "remote-docker",
			SourceRepository: "https://git.example/demo.git",
			SourceRevision: strings.Repeat("a", 40),
		},
	}
	if err := confirmStagedCoreGitCandidate(candidate, ""); err == nil {
		t.Fatal("missing approval consent accepted")
	}
	if err := confirmStagedCoreGitCandidate(candidate, strings.Repeat("b", 64)); err == nil {
		t.Fatal("wrong approval consent accepted")
	}
	exact := delivery.CandidateConsentDigest(candidate.Submission)
	if err := confirmStagedCoreGitCandidate(candidate, exact); err != nil {
		t.Fatalf("exact staged candidate consent rejected: %v", err)
	}
	if err := os.WriteFile(path, []byte("version: two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := confirmStagedCoreGitCandidate(candidate, exact); err == nil {
		t.Fatal("staged source mutation after confirmation was accepted")
	}
}
