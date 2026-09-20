package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

func inspectRepositorySource(ctx context.Context, source string) (repositoryinspect.Result, error) {
	if !looksLikeRemoteGitSource(source) {
		return repositoryinspect.Inspect(ctx, source)
	}
	if err := validateRemoteGitSource(source); err != nil {
		return repositoryinspect.Result{}, err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return repositoryinspect.Result{}, fmt.Errorf("remote repository inspection requires git: %w", err)
	}
	tempDir, err := os.MkdirTemp("", "baseharbor-inspect-*")
	if err != nil {
		return repositoryinspect.Result{}, fmt.Errorf("create temporary repository directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "--depth", "1", "--filter=blob:none", "--no-tags", "--", source, tempDir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(strings.ReplaceAll(stderr.String(), source, "<remote>"))
		if detail == "" {
			detail = err.Error()
		}
		return repositoryinspect.Result{}, fmt.Errorf("clone remote repository for read-only inspection: %s", detail)
	}
	result, err := repositoryinspect.Inspect(ctx, tempDir)
	if err != nil {
		return repositoryinspect.Result{}, err
	}
	result.Root = source
	return result, nil
}

func looksLikeRemoteGitSource(source string) bool {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "git@") {
		return true
	}
	for _, prefix := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(strings.ToLower(source), prefix) {
			return true
		}
	}
	return false
}

func validateRemoteGitSource(source string) error {
	if strings.HasPrefix(source, "git@") {
		return nil
	}
	parsed, err := url.Parse(source)
	if err != nil {
		return usageError("invalid remote Git URL", "Use a normal HTTPS/SSH Git URL or a local path.")
	}
	if parsed.User != nil {
		return usageError("remote Git URLs with embedded credentials are not accepted", "Use normal Git credential helpers, SSH agents or other Git authentication instead.")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "http", "ssh", "git":
		return nil
	default:
		return usageError("unsupported remote Git URL scheme "+parsed.Scheme, "Use HTTPS, SSH, git:// or a local path.")
	}
}
