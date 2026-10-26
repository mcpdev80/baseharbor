package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type fakeNativeCoreRuntime struct {
	bhruntime.RuntimeProvider
	containers       []bhruntime.RuntimeContainer
	image            bhruntime.ImageIdentity
	stopped, started int
}

func (f *fakeNativeCoreRuntime) StopProject(_ context.Context, _, _, _ string) error {
	f.stopped++
	return nil
}
func (f *fakeNativeCoreRuntime) UpProject(_ context.Context, _, _, _ string) error {
	f.started++
	return nil
}
func (f *fakeNativeCoreRuntime) ListRuntimeContainers(context.Context) ([]bhruntime.RuntimeContainer, error) {
	return f.containers, nil
}
func (f *fakeNativeCoreRuntime) ProjectServiceImageIdentity(context.Context, string, string) (bhruntime.ImageIdentity, error) {
	return f.image, nil
}

func (f *fakeNativeCoreRuntime) VerifyOwnedVolumeQuiesced(_ context.Context, project, volume string) error {
	if volume != "owned-core-postgres-data-1" {
		return errors.New("wrong recovery volume")
	}
	for _, c := range f.containers {
		if c.Project == project && c.Running {
			return errors.New("active volume consumer")
		}
	}
	return nil
}

func TestNativeCoreQuiesceRefusesActiveWriters(t *testing.T) {
	runtime := &fakeNativeCoreRuntime{containers: []bhruntime.RuntimeContainer{{Project: "owned-core", Service: "postgres-member-1", Running: true}}}
	ops := &coreNativeRuntimeOps{runtime: runtime, core: bhruntime.Files{Project: "owned-core", Compose: filepath.Join(t.TempDir(), "core.yaml"), Env: "core.env"}}
	if err := os.WriteFile(ops.core.Compose, []byte("services:\n  postgres-member-1:\n    volumes: [postgres-data-1:/var/lib/postgresql/data]\nvolumes:\n  postgres-data-1:\n    name: owned-core-postgres-data-1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	delta := coreupdate.Delta{Installed: coreupdate.Realization{Kind: coreupdate.SQL, Instance: "postgres-member-1"}}
	if err := ops.Quiesce(context.Background(), delta); err == nil {
		t.Fatal("active writer treated as quiesced")
	}
	if runtime.stopped != 1 {
		t.Fatal("Core stop not attempted")
	}
	runtime.containers[0].Running = false
	if err := ops.Quiesce(context.Background(), delta); err != nil {
		t.Fatal(err)
	}
}
func TestNativeCorePinnedImageVerificationRefusesDigestDrift(t *testing.T) {
	runtime := &fakeNativeCoreRuntime{image: bhruntime.ImageIdentity{Reference: "postgres:18", Digest: "sha256:" + strings.Repeat("a", 64)}}
	ops := &coreNativeRuntimeOps{runtime: runtime, core: bhruntime.Files{Project: "owned-core", Compose: "core.yaml", Env: "core.env"}, identity: identityprovider.KeycloakFiles{Project: "owned-identity"}}
	delta := coreupdate.Delta{Installed: coreupdate.Realization{Kind: coreupdate.SQL, Instance: "postgres-member-1", Digest: "sha256:" + strings.Repeat("a", 64)}, Desired: coreupdate.Desired{Kind: coreupdate.SQL, Digest: "sha256:" + strings.Repeat("b", 64)}}
	if err := ops.ReconcilePinned(context.Background(), delta); err == nil {
		t.Fatal("wrong running PG digest accepted")
	}
	runtime.image.Digest = delta.Desired.Digest
	if err := ops.ReconcilePinned(context.Background(), delta); err != nil {
		t.Fatal(err)
	}
	if runtime.started != 2 {
		t.Fatal("Core reconcile not executed")
	}
	if err := ops.ReconcileOriginal(context.Background(), delta); err == nil {
		t.Fatal("failed to detect old digest mismatch")
	}
}

func TestNativeCoreReceiptsPreserveEveryProviderStep(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	ops := &coreNativeRuntimeOps{release: "0.4.24", receiptPath: filepath.Join(dir, "receipts.json")}
	first := coreupdate.Delta{Installed: coreupdate.Realization{Kind: coreupdate.SQL, Installation: "core", Scope: "shared", Instance: "postgres-member-1", Owner: "baseharbor"}, Desired: coreupdate.Desired{Kind: coreupdate.SQL, Image: "postgres:18", Version: "18", Digest: "sha256:" + strings.Repeat("a", 64)}}
	second := coreupdate.Delta{Installed: coreupdate.Realization{Kind: coreupdate.Secrets, Installation: "core", Scope: "shared", Instance: "openbao-member-1", Owner: "baseharbor"}, Desired: coreupdate.Desired{Kind: coreupdate.Secrets, Image: "bao:2.7", Version: "2.7", Digest: "sha256:" + strings.Repeat("b", 64)}}
	if err := ops.Record(context.Background(), first, "verified"); err != nil {
		t.Fatal(err)
	}
	if err := ops.Record(context.Background(), second, "applying"); err != nil {
		t.Fatal(err)
	}
	journal, err := coreupdate.LoadJournal(ops.receiptPath, ops.release)
	if err != nil {
		t.Fatal(err)
	}
	if len(journal.Steps) != 2 || journal.Steps[coreupdate.JournalKey(first)] != "verified" || journal.Steps[coreupdate.JournalKey(second)] != "applying" {
		t.Fatalf("Core provider receipt lost prior migration evidence: %+v", journal.Steps)
	}
}

func TestNativeRecoveryUsesActualOwnerDataLayer(t *testing.T) {
	cases := []struct {
		kind               coreupdate.ProviderKind
		instance, expected string
	}{
		{coreupdate.SQL, "postgres-member-1", "postgres-member-1"},
		{coreupdate.Secrets, "openbao-member-1", "postgres-member-1"},
		{coreupdate.Identity, "keycloak-1", "keycloak-db"},
	}
	for _, tc := range cases {
		d := coreupdate.Delta{Installed: coreupdate.Realization{Kind: tc.kind, Instance: tc.instance}}
		if got := nativeRecoveryService(d); got != tc.expected {
			t.Errorf("%s expected datastore %s got %s", tc.instance, tc.expected, got)
		}
	}
	compose := filepath.Join("..", "..", "internal", "runtime", "assets", "compose-single.yaml")
	volume, err := coreupdate.ResolveOwnedServiceVolume(compose, "postgres-member-1", "bh-test")
	if err != nil || volume != "bh-test_postgres-data-1" {
		t.Fatalf("checked-in Core Postgres mount not recoverable: %q %v", volume, err)
	}
}

type keycloakVersionProbeRuntime struct {
	fakeNativeCoreRuntime
	versions map[string]string
}

func (r *keycloakVersionProbeRuntime) ProjectServiceImageIdentity(_ context.Context, _, service string) (bhruntime.ImageIdentity, error) {
	return bhruntime.ImageIdentity{Reference: "quay.io/keycloak/keycloak:" + r.versions[service], Digest: "sha256:" + strings.Repeat("a", 64)}, nil
}

func TestNativeKeycloakHAInventoryRejectsMixedMemberVersions(t *testing.T) {
	rt := &keycloakVersionProbeRuntime{versions: map[string]string{
		"keycloak-1": "26.3.3", "keycloak-2": "26.3.4", "keycloak-3": "26.3.3",
	}}
	for _, service := range []string{"keycloak-1", "keycloak-2", "keycloak-3"} {
		rt.containers = append(rt.containers, bhruntime.RuntimeContainer{Project: "owned-identity", Service: service, Running: true})
	}
	ops := &coreNativeRuntimeOps{runtime: rt, core: bhruntime.Files{HA: true}, identity: identityprovider.KeycloakFiles{Project: "owned-identity"}}
	mixed, err := ops.inspectNativeKeycloakMember(context.Background(), "keycloak-1")
	if err != nil || len(mixed.Members) != 3 || mixed.Members[1].Version != "26.3.4" {
		t.Fatalf("rolling Keycloak HA member versions must remain observable: %+v %v", mixed, err)
	}
	rt.versions["keycloak-2"] = "26.3.3"
	state, err := ops.inspectNativeKeycloakMember(context.Background(), "keycloak-1")
	if err != nil || len(state.Members) != 3 || state.Version != "26.3.3" {
		t.Fatalf("uniform healthy Keycloak HA members rejected: %+v %v", state, err)
	}
}
