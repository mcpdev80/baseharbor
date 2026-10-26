package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development"
)

func TestWorkspaceGitHumanOutputHidesGitInternalsByDefault(t *testing.T) {
	repos := []development.WorkspaceGitRepositoryStatus{{
		Source:           "api-source",
		State:            development.WorkspaceGitCurrent,
		Path:             "/home/dev/api",
		Branch:           "main",
		Upstream:         "origin/main",
		CurrentRevision:  "1111111111111111111111111111111111111111",
		TargetRevision:   "2222222222222222222222222222222222222222",
		DeclaredRef:      "main",
		DeclaredRevision: "1111111111111111111111111111111111111111",
	}}
	var out bytes.Buffer
	if err := writeWorkspaceGitStatus(&out, repos, false, false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, forbidden := range []string{"111111", "222222", "origin/main", "/home/dev/api", "Branch:", "Upstream:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("default output leaked %q:\n%s", forbidden, text)
		}
	}
	if !strings.Contains(text, "CURRENT") || !strings.Contains(text, "No new changes.") {
		t.Fatalf("missing human state language:\n%s", text)
	}
}

func TestWorkspaceGitVerboseOutputIncludesGitEvidence(t *testing.T) {
	repos := []development.WorkspaceGitRepositoryStatus{{
		Source:           "api-source",
		State:            development.WorkspaceGitUpdateAvailable,
		Path:             "/home/dev/api",
		Branch:           "main",
		Upstream:         "origin/main",
		CurrentRevision:  "1111111111111111111111111111111111111111",
		TargetRevision:   "2222222222222222222222222222222222222222",
		DeclaredRef:      "main",
		DeclaredRevision: "1111111111111111111111111111111111111111",
	}}
	var out bytes.Buffer
	if err := writeWorkspaceGitStatus(&out, repos, true, true); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, expected := range []string{
		"UPDATE AVAILABLE",
		"Path: /home/dev/api",
		"Branch: main",
		"Upstream: origin/main",
		"Current revision: 111111",
		"Upstream revision: 222222",
		"Declared ref: main",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("verbose output missing %q:\n%s", expected, text)
		}
	}
}

func TestWorkspaceGitPartialProgressHumanSummary(t *testing.T) {
	repos := []development.WorkspaceGitRepositoryStatus{
		{Source: "api-source", State: development.WorkspaceGitCurrent, Updated: true},
		{Source: "web-source", State: development.WorkspaceGitDirty, Blocker: "dirty_worktree", Problem: "repository contains local changes", NextAction: "run git status"},
	}
	var out bytes.Buffer
	if err := writeWorkspaceGitStatus(&out, repos, false, false); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, expected := range []string{
		"New changes were applied.",
		"ACTION REQUIRED",
		"1 repositories updated",
		"1 repositories need your attention",
		"Changes already applied were not rolled back.",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("partial-progress output missing %q:\n%s", expected, text)
		}
	}
}
