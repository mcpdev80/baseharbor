package main

import (
	"context"
	"crypto/sha256"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func qualifyBackendPlacementRoundTrip(t *testing.T, ctx context.Context, provider bhruntime.RuntimeProvider, original resolvedApplication, shared application.SharedBackendInventory, service string, sharedEnv, originalCredentials [32]byte, sharedVolumes []bhruntime.ProjectResource) {
	t.Helper()
	kind := capability.ProviderPostgreSQL
	if service == "cache" {
		kind = capability.ProviderValkey
	}
	var destinationCredentials [32]byte
	var destinationVolumes []bhruntime.ProjectResource
	for phase, scope := range []string{"application", "shared", "application", "shared"} {
		t.Setenv(application.ProviderScopeEnv(kind), scope)
		current := applySelectiveIdentityIntent(t, ctx)
		if current.Manifest.ApplicationID != original.Manifest.ApplicationID {
			t.Fatal("placement replacement changed application identity")
		}
		status, _, err := collectResolvedApplicationStatus(ctx, current)
		if err != nil || !status.Ready {
			t.Fatalf("placement=%s is not verified ready: %v", scope, err)
		}
		data, err := os.ReadFile(shared.Files.Env)
		if err != nil || sha256.Sum256(data) != sharedEnv {
			t.Fatal("placement replacement changed original provider credentials")
		}
		actualShared, err := provider.InspectProjectResources(ctx, shared.Files.Project, shared.Resources)
		if err != nil {
			t.Fatal("cannot inspect original retained provider")
		}
		for _, volume := range sharedVolumes {
			if !application.ResourceNamedExists(actualShared, volume) {
				t.Fatal("placement replacement lost original shared volume")
			}
		}
		files := application.RuntimeFilesFor(current.Store, current.Manifest)
		owners, err := nativeBackendOwnerIntents(current)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := nativeBackendOwnedResourceUnion(owners, files, nil)
		if err != nil {
			t.Fatal(err)
		}
		actual, err := provider.InspectProjectResources(ctx, files.Project, expected)
		if err != nil {
			t.Fatal("cannot inspect retained application destination")
		}
		if phase == 0 {
			for _, resource := range actual {
				if resource.Kind == "volume" {
					destinationVolumes = append(destinationVolumes, resource)
				}
			}
			if len(destinationVolumes) == 0 {
				t.Fatal("native destination has no owned persistent volume")
			}
		}
		for _, volume := range destinationVolumes {
			if !application.ResourceNamedExists(actual, volume) {
				t.Fatal("return to shared placement lost destination volume")
			}
		}
		if scope == "shared" {
			qualifySelectedBackendData(t, ctx, provider, current, shared, service, 2, &originalCredentials)
		} else {
			resources, err := application.NativeBackendResources(current.Manifest, files)
			if err != nil {
				t.Fatal(err)
			}
			native := application.SharedBackendInventory{Files: application.SharedBackendFiles{Project: files.Project, Compose: files.Compose, Env: files.Env}, Services: map[capability.ProviderKind][]string{}}
			for _, resource := range resources {
				if resource.Kind == "container" {
					name := strings.TrimSuffix(strings.TrimPrefix(resource.Name, files.Project+"-"), "-1")
					native.Services[kind] = append(native.Services[kind], name)
				}
			}
			if phase == 0 {
				qualifySelectedBackendData(t, ctx, provider, current, native, service, -1, &destinationCredentials)
				qualifySelectedBackendData(t, ctx, provider, current, native, service, 0, &destinationCredentials)
			} else {
				qualifySelectedBackendData(t, ctx, provider, current, native, service, 2, &destinationCredentials)
			}
		}
		// Repeat the already verified placement, without reseeding either store.
		applySelectiveIdentityIntent(t, ctx)
		t.Logf("Backend placement=%s phase=%d: actual authenticated destination verified; original and destination volumes/credentials retained; no automatic data migration; repeated apply passed.", scope, phase)
	}
}
