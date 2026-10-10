package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestOwnedTaggedImageRequiresMatchingRepositoryAndDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	path := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(path, []byte("services:\n  keycloak-1:\n    image: quay.io/keycloak/keycloak:26.8.0@"+digest+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	good := bhruntime.ImageIdentity{Reference: "quay.io/keycloak/keycloak@" + digest, Digest: "quay.io/keycloak/keycloak@" + digest}
	ref, err := ownedTaggedImage(good, path, "keycloak-1")
	if err != nil || ref != "quay.io/keycloak/keycloak:26.8.0" {
		t.Fatalf("lost canonical image tag: %q %v", ref, err)
	}
	for _, image := range []bhruntime.ImageIdentity{{Reference: "foreign/keycloak@" + digest, Digest: digest}, {Reference: good.Reference, Digest: "sha256:" + strings.Repeat("b", 64)}} {
		if _, err := ownedTaggedImage(image, path, "keycloak-1"); err == nil {
			t.Fatal("accepted foreign runtime identity")
		}
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ownedTaggedImage(good, path, "keycloak-1"); err == nil {
		t.Fatal("accepted unprotected source")
	}
}

func TestNativeKeycloakMinorRequiresSingleOwnedMigration(t *testing.T) {
	for _, ha := range []bool{false, true} {
		ops := coreNativeRuntimeOps{core: bhruntime.Files{HA: ha}}
		err := ops.keycloakUpgradePath(t.Context(), "26.7.5", "26.8.0")
		if (err == nil) == ha {
			t.Fatal("minor migration admission differs from safe topology")
		}
		if err := ops.keycloakUpgradePath(t.Context(), "26.8.0", "27.0.0"); err == nil {
			t.Fatal("major upgrade admitted")
		}
	}
}

func TestOwnedTaggedImageAcceptsOnlyCanonicalDockerHubAliases(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	path := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(path, []byte("services:\n  postgres-member-1:\n    image: docker.io/library/postgres:18.0-alpine@"+digest+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, repository := range []string{"postgres", "library/postgres", "docker.io/library/postgres", "index.docker.io/library/postgres", "registry-1.docker.io/library/postgres"} {
		ref, err := ownedTaggedImage(bhruntime.ImageIdentity{Reference: repository + "@" + digest, Digest: repository + "@" + digest}, path, "postgres-member-1")
		if err != nil || ref != "docker.io/library/postgres:18.0-alpine" {
			t.Fatalf("Docker Hub alias %q not resolved from owned immutable declaration: %q %v", repository, ref, err)
		}
	}
	if _, err := ownedTaggedImage(bhruntime.ImageIdentity{Reference: "quay.io/library/postgres@" + digest, Digest: digest}, path, "postgres-member-1"); err == nil {
		t.Fatal("foreign registry admitted as Docker Hub alias")
	}
}
