package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/localhosts"
)

func TestWorkstationHostsOwnershipAndRollback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	state, path, err := loadWorkstationHosts("dev-docker-1")
	if err != nil {
		t.Fatal(err)
	}
	original := writeSystemHosts
	t.Cleanup(func() { writeSystemHosts = original })
	current := "127.0.0.1 localhost\n"
	writeSystemHosts = func(_ context.Context, change localhosts.Change) error {
		next, err := localhosts.Rewrite(current, change)
		if err == nil {
			current = next
		}
		return err
	}
	marker := "target=dev-docker-1 installation=fc204331-0c16-44ad-9a10-a7c71df4919b"
	block, _ := localhosts.BlockWithLabel(workstationHostsID(state.Target), marker, "192.0.2.1", []string{"demo.example.test"})
	if err := changeWorkstationHosts(context.Background(), state, block, path, true, marker); err != nil {
		t.Fatal(err)
	}
	state, _, err = loadWorkstationHosts(state.Target)
	if err != nil || !state.Enabled || state.Block != block || state.Marker != marker || !strings.Contains(current, "target=dev-docker-1 installation=fc204331") {
		t.Fatalf("ownership not persisted: %v", err)
	}
	if err := removeWorkstationHosts(context.Background(), state.Target, os.Stderr); err != nil {
		t.Fatal(err)
	}
	if current != "127.0.0.1 localhost\n" {
		t.Fatal("cleanup altered unrelated contents")
	}
	// A nonempty directory cannot be replaced by the atomic ownership file.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "keep"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	state.Block = ""
	if err := changeWorkstationHosts(context.Background(), state, block, blocked, true); err == nil {
		t.Fatal("expected ownership write failure")
	}
	if current != "127.0.0.1 localhost\n" {
		t.Fatal("failed persistence did not roll back")
	}
}

func TestWorkstationHostsFailedWriterDoesNotEnable(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	original := writeSystemHosts
	t.Cleanup(func() { writeSystemHosts = original })
	writeSystemHosts = func(context.Context, localhosts.Change) error { return errors.New("permission denied") }
	state, path, err := loadWorkstationHosts("dev-docker-1")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := localhosts.Block(workstationHostsID(state.Target), "192.0.2.1", []string{"demo.example.test"})
	if err := changeWorkstationHosts(context.Background(), state, block, path, true); err == nil {
		t.Fatal("expected failure")
	}
	loaded, _, _ := loadWorkstationHosts(state.Target)
	if loaded.Enabled {
		t.Fatal("failed write enabled management")
	}
}

func TestWorkstationCAReferenceCleanupSelection(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_STATE_DIR", "")
	// No certificate parsing is needed for a previously cleaned connection.
	marker, err := workstationTrustMarker("second")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(marker))
	profile := filepath.Join(root, "remote-connections", "second", "profile.json")
	if err := os.MkdirAll(filepath.Dir(profile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile, []byte("invalid retained profile"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := otherConnectionReferencesCA(root, "first", "fingerprint"); err == nil {
		t.Fatal("invalid active profile must preserve CA")
	}
	if err := setWorkstationTrustActive("second", false); err != nil {
		t.Fatal(err)
	}
	shared, err := otherConnectionReferencesCA(root, "first", "fingerprint")
	if err != nil || shared {
		t.Fatalf("cleaned connection still retains CA: %v", err)
	}
	body, _ := os.ReadFile(marker)
	var parsed map[string]bool
	if err := json.Unmarshal(body, &parsed); err != nil || parsed["active"] {
		t.Fatal("cleanup state invalid")
	}
}

func TestImportedHostsDefaultOffAndExplicitOptIn(t *testing.T) {
	target := configureTestTarget(t)
	oldRead, oldWrite, oldInput := readWorkstationHostsMetadata, writeSystemHosts, onboardingInput
	t.Cleanup(func() {
		readWorkstationHostsMetadata = oldRead
		writeSystemHosts = oldWrite
		onboardingInput = oldInput
	})
	ctx := withTargetOverride(context.Background(), target.Name)
	for _, tc := range []struct {
		name, input             string
		options                 onboardTrustOptions
		noninteractive, enabled bool
	}{
		{name: "default-no", input: "\n"}, {name: "decline", input: "n\n"}, {name: "eof"},
		{name: "no-input", input: "yes\n", noninteractive: true},
		{name: "skip", input: "yes\n", options: onboardTrustOptions{skipHosts: true}},
		{name: "explicit", options: onboardTrustOptions{hosts: true}, noninteractive: true, enabled: true},
		{name: "interactive", input: "yes\n", enabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, path, err := loadWorkstationHosts(target.Name)
			if err != nil {
				t.Fatal(err)
			}
			_ = os.Remove(path)
			reads, writes := 0, 0
			readWorkstationHostsMetadata = func(context.Context, deployment.ResolvedTarget) (*developmentHosts, error) {
				reads++
				return &developmentHosts{Address: "192.0.2.3", Names: []string{"demo.example.test"}, InstallationID: "fc204331-0c16-44ad-9a10-a7c71df4919b"}, nil
			}
			writeSystemHosts = func(context.Context, localhosts.Change) error { writes++; return nil }
			onboardingInput = strings.NewReader(tc.input)
			var out bytes.Buffer
			callCtx := cli.WithOutputOptions(ctx, cli.OutputOptions{NonInteractive: tc.noninteractive})
			if err := offerImportedHosts(callCtx, bufio.NewReader(onboardingInput), &out, tc.options); err != nil {
				t.Fatal(err)
			}
			if (writes == 1) != tc.enabled {
				t.Fatalf("unexpected hosts writes: %d", writes)
			}
			state, _, _ := loadWorkstationHosts(target.Name)
			if state.Enabled != tc.enabled {
				t.Fatal("wrong persisted opt-in")
			}
			if !tc.enabled {
				refreshWorkstationHosts(callCtx, target, &out)
				if writes != 0 {
					t.Fatal("deployment changed hosts before opt-in")
				}
			}
			if tc.noninteractive && !tc.options.hosts && reads != 0 {
				t.Fatal("default no-input unexpectedly inspected routes")
			}
		})
	}
	if _, _, err := stripOnboardTrustOptions([]string{"--configure-hosts", "--skip-hosts"}); err == nil {
		t.Fatal("conflicting opt-ins accepted")
	}
}
