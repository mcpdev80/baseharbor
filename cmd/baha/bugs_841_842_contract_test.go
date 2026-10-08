package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMachineCollectionEnvelopesNeverUseNull(t *testing.T) {
	for name, result := range map[string]any{
		"provider":     normalizedProviderList(nil),
		"connectivity": normalizedConnectivityList(nil),
	} {
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		if m["contract_version"] != "v1" {
			t.Fatalf("%s missing version: %s", name, data)
		}
		key := "providers"
		if name == "connectivity" {
			key = "rules"
		}
		values, ok := m[key].([]any)
		if !ok || len(values) != 0 {
			t.Fatalf("%s missing non-null collection: %s", name, data)
		}
	}
}

func TestNestedWorkloadMachineShape(t *testing.T) {
	b, err := json.Marshal(applicationOverview{ContractVersion: "v1", Postgres: []overviewResource{}, Valkey: []overviewResource{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{`"workload":{"found":false`, `"repository_root":""`, `"services":[]`, `"build_drift":[]`} {
		if !strings.Contains(string(b), fragment) {
			t.Fatalf("missing %q in %s", fragment, b)
		}
	}
	if strings.Contains(string(b), `"RepositoryRoot"`) || strings.Contains(string(b), `"Services":null`) {
		t.Fatal(string(b))
	}
}

func TestBackupInGitRepoExplicitPathWarnsAndDefaultRejects(t *testing.T) {
	worktree := t.TempDir()
	if err := os.Mkdir(filepath.Join(worktree, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(worktree)
	t.Setenv("XDG_DATA_HOME", filepath.Join(worktree, "backups-data"))
	_, err := defaultGuidedBackupPath("webshop", "dev", time.Now())
	if err == nil {
		t.Fatal("default backup must not enter Git worktree even when XDG data location does")
	}
	path := filepath.Join(worktree, "manual.bhbackup")
	var buf bytes.Buffer
	reportGitBackupRisk(&buf, path)
	if !strings.Contains(buf.String(), "WARNING") || !strings.Contains(buf.String(), "git add -A") {
		t.Fatalf("missing explicit archive warning: %s", buf.String())
	}
	if backupOutputFlag([]string{"--output", path}) != path {
		t.Fatal("explicit output flag lost")
	}
	if backupOutputFlag([]string{"--output=" + path}) != path {
		t.Fatal("equals output flag lost")
	}
}
