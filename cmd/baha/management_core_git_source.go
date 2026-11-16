package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
)

// verifyCoreGitCheckout verifies an already registered Core-owned Git checkout.
// No client-provided URL is fetched and no workstation path is accepted. The
// normal application lifecycle keeps ownership of the actual runtime mutation.
func verifyCoreGitCheckout(ctx context.Context, source development.SourceDefinition, recorded deployment.DeploymentSource) error {
	if source.Type != development.SourceRepository {
		return fmt.Errorf("Core-side OCI image admission is not yet available")
	}
	if recorded.Kind != "repository" || recorded.Repository == "" {
		return fmt.Errorf("Core has no registered repository checkout for this application")
	}
	root, err := filepath.EvalSymlinks(recorded.Repository)
	if err != nil {
		return fmt.Errorf("resolve registered Core source checkout: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("registered Core source checkout is unavailable")
	}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(checkCtx, "git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
		data, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("verify registered Git checkout: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil || filepath.Clean(top) != filepath.Clean(root) {
		return fmt.Errorf("registered Core source is not the Git worktree root")
	}
	origin, err := git("config", "--get", "remote.origin.url")
	if err != nil || origin != source.Repository {
		return fmt.Errorf("registered Core Git origin does not match the authorized source")
	}
	head, err := git("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || head != source.Ref {
		return fmt.Errorf("registered Core Git checkout is not at the selected immutable commit")
	}
	status, err := git("status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || status != "" {
		return fmt.Errorf("registered Core Git checkout has uncommitted or untracked source changes")
	}
	return nil
}
