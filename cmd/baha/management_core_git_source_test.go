package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestVerifyCoreGitCheckoutRequiresExactCleanRegisteredSource(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init")
	git("remote", "add", "origin", "https://git.example/demo.git")
	if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte("app: demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "baseharbor.yaml")
	git("-c", "user.name=Test", "-c", "user.email=test@example.org", "commit", "-m", "fixture")
	commit := git("rev-parse", "HEAD")
	source := development.SourceDefinition{
		ID: "app", Type: development.SourceRepository,
		Repository: "https://git.example/demo.git", Ref: commit,
	}
	recorded := deployment.DeploymentSource{Kind: "repository", Repository: root}
	if err := verifyCoreGitCheckout(context.Background(), source, recorded); err != nil {
		t.Fatalf("valid Core-registered Git checkout refused: %v", err)
	}
	wrongCommit := source
	wrongCommit.Ref = strings.Repeat("a", 40)
	if err := verifyCoreGitCheckout(context.Background(), wrongCommit, recorded); err == nil {
		t.Fatal("wrong Git revision accepted")
	}
	wrongOrigin := source
	wrongOrigin.Repository = "https://git.example/other.git"
	if err := verifyCoreGitCheckout(context.Background(), wrongOrigin, recorded); err == nil {
		t.Fatal("foreign Git origin accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "extra.txt"), []byte("untracked"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyCoreGitCheckout(context.Background(), source, recorded); err == nil {
		t.Fatal("dirty Git checkout accepted")
	}
	if err := os.Remove(filepath.Join(root, "extra.txt")); err != nil {
		t.Fatal(err)
	}
	if err := verifyCoreGitCheckout(context.Background(), source,
		deployment.DeploymentSource{Kind: "managed", Repository: root}); err == nil {
		t.Fatal("unregistered managed source was accepted as Git")
	}
}
