package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Both consumers use their actual persisted bindings. Removing one consumer
// must preserve the other's authenticated data access, not merely a container.
func qualifySharedBackendConsumers(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, first resolvedApplication, inventory application.SharedBackendInventory, service string, originalEnv, firstIdentity [32]byte, volumes []bhruntime.ProjectResource) {
	t.Helper()
	firstRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	initialEnvironment, err := os.ReadFile(inventory.Files.Env)
	if err != nil || sha256.Sum256(initialEnvironment) != originalEnv {
		t.Fatal("shared backend initial provider configuration changed")
	}
	siblingRoot := t.TempDir()
	write := func(root, name string, enabled bool) {
		content := fmt.Sprintf("version: 1\napp:\n  name: %s\n  environment: dev\nservices:\n  %s: %t\n", name, service, enabled)
		if err := os.WriteFile(filepath.Join(root, "baseharbor.yaml"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(siblingRoot, "compose.yaml"), []byte(corelessHTTPWorkload), 0600); err != nil {
		t.Fatal(err)
	}
	write(siblingRoot, "backend-sibling-"+service, true)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(machineNoninteractiveContext(context.Background()), 2*time.Minute)
		defer cancel()
		inSelectiveIdentityRepository(t, siblingRoot, func() {
			if err := destroySelectiveApplicationWithConsent(cleanupCtx); err != nil {
				t.Errorf("shared backend sibling cleanup: %v", err)
			}
		})
	})
	var sibling resolvedApplication
	var siblingIdentity [32]byte
	inSelectiveIdentityRepository(t, siblingRoot, func() {
		var output bytes.Buffer
		if err := runWithIO(ctx, []string{"app", "init", "--quick", "--json"}, &output, &output); err != nil {
			t.Fatalf("initialize backend sibling: %v\n%s", err, output.String())
		}
		applySelectiveIdentityIntent(t, ctx)
		var err error
		sibling, err = resolveApplication(ctx, application.Store{}, nil, "status")
		if err != nil {
			t.Fatal(err)
		}
		qualifySelectedBackendData(t, ctx, provider, sibling, inventory, service, 0, &siblingIdentity)
	})
	if sibling.Manifest.ApplicationID == first.Manifest.ApplicationID {
		t.Fatal("independent backend consumers share a system identity")
	}
	// The canonical cache environment adds the second consumer's own credential
	// entries. Preserve every existing entry while allowing those additions;
	// subsequent retirement/re-add must preserve this complete two-app snapshot.
	expandedEnvironment, err := os.ReadFile(inventory.Files.Env)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]bool{}
	for _, line := range strings.Split(string(expandedEnvironment), "\n") {
		entries[line] = true
	}
	for _, line := range strings.Split(string(initialEnvironment), "\n") {
		if !entries[line] {
			t.Fatal("new consumer replaced retained provider configuration")
		}
	}
	originalEnv = sha256.Sum256(expandedEnvironment)
	roots := []string{firstRoot, siblingRoot}
	apps := []resolvedApplication{first, sibling}
	identities := []*[32]byte{&firstIdentity, &siblingIdentity}
	for phase, changed := range []int{0, 1, 0, 1} {
		enabled := phase >= 2
		write(roots[changed], apps[changed].Manifest.Name, enabled)
		inSelectiveIdentityRepository(t, roots[changed], func() {
			current := applySelectiveIdentityIntent(t, ctx)
			if current.Manifest.ApplicationID != apps[changed].Manifest.ApplicationID {
				t.Fatal("shared backend change replaced consumer identity")
			}
			apps[changed] = current
		})
		inventory, err = application.ExistingSharedBackendInventoryAt(first.TargetStateRoot, first.Target.Name, "core")
		if err != nil || inventory.CoreSQL != nil {
			t.Fatalf("shared standalone backend ownership: %v", err)
		}
		data, err := os.ReadFile(inventory.Files.Env)
		if err != nil || sha256.Sum256(data) != originalEnv {
			t.Fatal("shared backend change replaced retained provider configuration")
		}
		actual, err := provider.InspectProjectResources(ctx, inventory.Files.Project, inventory.Resources)
		if err != nil {
			t.Fatal(err)
		}
		for _, volume := range volumes {
			if !application.ResourceNamedExists(actual, volume) {
				t.Fatal("last-consumer removal lost retained backend data volume")
			}
		}
		needed := phase != 1
		assertSelectedBackendNativeInventory(t, ctx, provider, apps[0], inventory, service, phase+3, needed, len(volumes), apps[1])
		for index := range apps {
			active := (index == 0 && phase >= 2) || (index == 1 && (phase == 0 || phase == 3))
			if !active {
				continue
			}
			inSelectiveIdentityRepository(t, roots[index], func() {
				current, err := resolveApplication(ctx, application.Store{}, nil, "status")
				if err != nil {
					t.Fatal(err)
				}
				status, _, err := collectResolvedApplicationStatus(ctx, current)
				if err != nil || !status.Ready {
					t.Fatalf("shared backend consumer=%d phase=%d readiness: %+v %v", index, phase, status, err)
				}
				qualifySelectedBackendData(t, ctx, provider, current, inventory, service, 2, identities[index])
			})
		}
		t.Logf("shared backend=%s phase=%d provider needed=%t; active consumers retain authenticated data", service, phase, needed)
	}
}
