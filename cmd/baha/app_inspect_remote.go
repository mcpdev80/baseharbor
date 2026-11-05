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

	"github.com/mcpdev80/baseharbor/internal/credential/native"
	repositoryinspect "github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

// cloneNonInteractive clones source with native authentication only. When a
// native source (git credential helper or ssh agent) can service the host, it
// starts git non-interactively so no credential prompt is possible and the
// read-only inspection proceeds; otherwise it returns false so the caller falls
// back to the standard clone. The native result is found-only: it never
// reports that authentication succeeded.
func cloneNonInteractive(ctx context.Context, out io.Writer, source string) (usedNative bool, err error) {
	host := native.HostFromURL(source)
	if host == "" {
		return false, nil
	}
	nativeHit, herr := native.PreInteractiveNoPrompt(ctx, host)
	if herr != nil {
		if out != nil {
			fmt.Fprintf(out, "[info] native authentication unavailable for %s: %v\n", source, herr)
		}
		return false, nil
	}
	if !nativeHit {
		return false, nil
	}
	if out != nil {
		fmt.Fprintf(out, "[info] reusing native authentication for %s (no interactive prompt)\n", source)
	}
	return true, nil
}

func inspectRepositorySource(ctx context.Context, source string, out io.Writer) (repositoryinspect.Result, error) {
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
	if nativeUsed, nerr := cloneNonInteractive(ctx, out, source); nativeUsed {
		cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")
	} else if nerr != nil {
		return repositoryinspect.Result{}, nerr
	}
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
