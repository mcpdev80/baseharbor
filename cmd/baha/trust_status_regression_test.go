package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrustStatusAbsentCoreInsideAndOutsideRepository(t *testing.T) {
	for _, repo := range []bool{false, true} {
		t.Run(map[bool]string{false: "outside", true: "inside"}[repo], func(t *testing.T) {
			t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
			configureTestTarget(t)
			t.Chdir(t.TempDir())
			if repo {
				if err := os.WriteFile("baseharbor.yaml", []byte("name: example\nenvironment: dev\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, format := range []string{"human", "plain", "json"} {
				ctx := context.Background()
				var args []string
				if format == "plain" {
					ctx = cli.WithOutputOptions(ctx, cli.OutputOptions{Plain: true})
				}
				if format == "json" {
					args = []string{"--json"}
				}
				var out, errOut bytes.Buffer
				if err := trustStatusCommand().Run(ctx, args, &out, &errOut); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(out.String(), "stat ") || strings.Contains(out.String(), "compose.yaml") {
					t.Fatalf("raw filesystem error: %s", out.String())
				}
				if format == "json" {
					var result managedTrustResult
					if err := json.Unmarshal(out.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if result.CoreState != "not_installed" || result.Next == "" {
						t.Fatalf("invalid result %+v", result)
					}
				} else if !strings.Contains(out.String(), "not installed") {
					t.Fatalf("missing state: %s", out.String())
				}
			}
		})
	}
}

func TestTrustCoreStateBeforeAfterDestroyAndIncomplete(t *testing.T) {
	target := configureTestTarget(t)
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "topology.json"), []byte(`{"version":1,"ha":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"compose.yaml", "runtime.env"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	state, err := managedTrustCoreState(context.Background())
	if err != nil || state != "installed" {
		t.Fatalf("before: %s %v", state, err)
	}
	if err := os.Remove(filepath.Join(root, "runtime.env")); err != nil {
		t.Fatal(err)
	}
	state, err = managedTrustCoreState(context.Background())
	if err != nil || state != "incomplete" {
		t.Fatalf("partial: %s %v", state, err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	state, err = managedTrustCoreState(context.Background())
	if err != nil || state != "not_installed" {
		t.Fatalf("after: %s %v", state, err)
	}
}

func TestTrustStatusRetainsOwnedRecordsWithoutCore(t *testing.T) {
	root := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", root)
	configureTestTarget(t)
	record := `{"version":1,"anchors":[{"fingerprint":"abcdef","backend":"linux-update-ca-certificates","path":"` + filepath.Join(root, "missing.crt") + `","installed_at":"2026-10-08T12:00:00Z"}]}`
	path := filepath.Join(root, "host-trust.json")
	if err := os.WriteFile(path, []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := trustStatusCommand().Run(context.Background(), nil, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "PRESERVED  abcdef") || !strings.Contains(out.String(), "missing") {
		t.Fatalf("missing retained anchor: %s", out.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != record {
		t.Fatal("read-only status changed ownership records")
	}
}

func TestTrustStatusUnreadableStateNotReportedAbsent(t *testing.T) {
	target := configureTestTarget(t)
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(root), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, nil, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = inspectManagedTrust(context.Background())
	if err == nil || strings.Contains(err.Error(), root) {
		t.Fatalf("unexpected I/O handling: %v", err)
	}
}
