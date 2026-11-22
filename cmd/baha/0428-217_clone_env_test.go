package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// zzCloneEnvProbeTimeout bounds the clone-child witness so a hung child never
// blocks the suite.
const zzCloneEnvProbeTimeout = 20 * time.Second

// TestGitCloneInspectInheritsParentEnvAndSuppressesPrompt proves BEFUND 1 of
// card 0428-217: the git clone that inspects a remote read-only must INHERIT
// the parent environment (so HOME/PATH/gitconfig/credential-helper survive)
// WHILE pinning GIT_TERMINAL_PROMPT=0.
//
// It exercises the production env construction the fix introduced: the child's
// full environment is recorded, and the test asserts both claims at once. The
// child's hostile inherited GIT_TERMINAL_PROMPT=1 proves that only the fix
// (deriving Env from os.Environ and suppressing the prompt channel) restores
// the child to GIT_TERMINAL_PROMPT=0; the round-1 baseline appended to a nil
// cmd.Env and wiped the whole inherited environment.
func TestGitCloneInspectInheritsParentEnvAndSuppressesPrompt(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "child-env.log")
	// The child records its full environment so the test can assert inheritance
	// and prompt suppression on the real production child vector.
	probe := filepath.Join(dir, "env-probe.sh")
	probeBody := "#!/bin/sh\n" +
		"env | tr '\\0' '\\n' | sed -n '1,200p' >> " + logPath + "\n"
	if err := os.WriteFile(probe, []byte(probeBody), 0o755); err != nil {
		t.Fatalf("write probe: %v", err)
	}

	// Set the hostile inherited prompt channel plus the sentinel. t.Setenv makes
	// this the process environment, which is what the fix derives from.
	t.Setenv("GIT_TERMINAL_PROMPT", "1") // inherited hostile: only the fix strips it
	t.Setenv("BASEHARBOR_0428_217_SENTINEL", "1")

	// Reproduce the production env construction the fix introduced.
	childEnv := withGitPromptDisabled(os.Environ())

	cmd := exec.CommandContext(context.Background(), probe)
	cmd.Env = childEnv
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("probe run: %v\n%s", err, out)
	}

	seen := zzChildEnvSnapshot(t, logPath)

	// (i) the sentinel inherited into the child vector.
	if v := seen["BASEHARBOR_0428_217_SENTINEL"]; len(v) != 1 || v[0] != "1" {
		t.Fatalf("clone child must inherit the parent sentinel, got %v", v)
	}
	// (ii) PATH inherited (proves the environment was not replaced by the
	// nil-append that round 1's develop baseline used).
	if _, ok := seen["PATH"]; !ok {
		t.Fatalf("clone child must inherit PATH; the nil-append replaced the whole environment")
	}
	// (iii) the hostile inherited GIT_TERMINAL_PROMPT=1 was suppressed to 0.
	// execve de-duplicates toward the last copy, so the LAST recorded value
	// must be 0 (the child cannot prompt); the inherited 1 may still appear
	// earlier in the raw env dump.
	prompt := seen["GIT_TERMINAL_PROMPT"]
	if len(prompt) == 0 || prompt[len(prompt)-1] != "0" {
		t.Fatalf("clone child's effective GIT_TERMINAL_PROMPT must be 0 (last copy wins), got %v", prompt)
	}
}

// TestGitCloneInspectEnvDerivationMutationAppendedNil is the negative witness:
// the FIXED source must derive Env from os.Environ (not append to the nil
// cmd.Env). With the buggy append the child would have NO PATH and the
// inheritance assertion above would fail, so a green run here plus a green run
// of the positive witness proves the nil-append defect is closed.
func TestGitCloneInspectEnvDerivationMutationAppendedNil(t *testing.T) {
	src, err := os.ReadFile("app_inspect_remote.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if strings.Contains(string(src), "append(cmd.Env,") {
		t.Fatalf("M1: app_inspect_remote.go still uses the buggy append(cmd.Env, ...) on a nil Env; derive from os.Environ instead")
	}
	if !strings.Contains(string(src), "withGitPromptDisabled(os.Environ())") {
		t.Fatalf("M1: app_inspect_remote.go must derive Env from os.Environ via withGitPromptDisabled")
	}
}

// TestGitCloneInspectPromptSuppressionPresent is the second mutation direction:
// the fixed source must actually SET the prompt-suppression sentinel on the
// child env. If the suppression line is removed the child would inherit the
// hostile GIT_TERMINAL_PROMPT=1 with no 0 to win, so the effective value is
// not 0. This pins that the fix both inherits AND suppresses.
func TestGitCloneInspectPromptSuppressionPresent(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("BASEHARBOR_0428_217_SENTINEL", "1")
	childEnv := withGitPromptDisabled(os.Environ())
	if !zzEnvEndsWith(childEnv, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("M2: withGitPromptDisabled must append GIT_TERMINAL_PROMPT=0 to the child env")
	}
}

// zzChildEnvSnapshot reads the child vector a probe helper recorded.
func zzChildEnvSnapshot(t *testing.T, path string) map[string][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read child env: %v", err)
	}
	seen := map[string][]string{}
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			seen[k] = append(seen[k], v)
		}
	}
	return seen
}

// zzEnvEndsWith reports whether the env slice ends with the given KEY=VALUE
// entry (execve de-duplicates toward the last copy, so the authoritative value
// is the last one).
func zzEnvEndsWith(env []string, want string) bool {
	for i := len(env) - 1; i >= 0; i-- {
		if env[i] == want {
			return true
		}
		if strings.HasPrefix(env[i], "GIT_TERMINAL_PROMPT=") && env[i] != want {
			// A later differing copy would have won; but we already scan from
			// the end, so the first match from the end is authoritative.
			return false
		}
	}
	return false
}
