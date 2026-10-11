package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderMaterialInventoryListsUnknownEntriesWithoutAdoptionOrSecretExposure(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	protected := filepath.Join(root, "runtime.env")
	readable := filepath.Join(root, "public.pem")
	foreign := filepath.Join(t.TempDir(), "foreign-key")
	linked := filepath.Join(root, "linked-key")
	for _, path := range []string{protected, readable, foreign} {
		if err := os.WriteFile(path, []byte("retained-sensitive-value"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(readable, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, linked); err != nil {
		t.Fatal(err)
	}
	materials, complete, err := observeOwnedProviderFiles("existing-owner", "isolated", "identity/shared", root)
	if err != nil || complete || len(materials) != 3 {
		t.Fatalf("unknown material omitted or called complete: %d %t %v", len(materials), complete, err)
	}
	for _, material := range materials {
		if material.Location == nil || material.Evidence == "" {
			t.Fatal("material lacks location/reason")
		}
		if material.Location.Reference == protected {
			if !material.Verified || material.Ownership != "baseharbor" {
				t.Fatal("protected owner material was not inventoried")
			}
		} else if material.Verified || material.Ownership != "" || material.Kind != "unknown" {
			t.Fatal("unverified entry was adopted or reported verified")
		}
	}
	encoded, err := json.Marshal(materials)
	if err != nil || strings.Contains(string(encoded), "retained-sensitive-value") || strings.Contains(string(encoded), foreign) {
		t.Fatal("material inventory exposed contents or followed the foreign link")
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "retained-sensitive-value" {
		t.Fatal("inventory modified foreign material")
	}
}

func TestProviderMaterialInventoryReportsSkippedSubtreeAtItsActualLocation(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	unprotected := filepath.Join(root, "unprotected")
	if err := os.Mkdir(unprotected, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unprotected, "private-key"), []byte("never-inspected"), 0600); err != nil {
		t.Fatal(err)
	}
	materials, complete, err := observeOwnedProviderFiles("existing-owner", "isolated", "identity/shared", root)
	if err != nil || complete || len(materials) != 1 || materials[0].Location.Reference != unprotected || materials[0].Verified {
		t.Fatalf("skipped subtree has no bounded unknown-location receipt: %+v %t %v", materials, complete, err)
	}
}

func TestProviderMaterialInventoryDoesNotAdoptProtectedHardLinkToForeignMaterial(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "recovery-key")
	if err := os.WriteFile(foreign, []byte("foreign-recovery-material"), 0600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "private-key")
	if err := os.Link(foreign, linked); err != nil {
		t.Fatal(err)
	}
	materials, complete, err := observeOwnedProviderFiles("canonical-owner", "isolated", "identity/shared", root)
	if err != nil || complete || len(materials) != 1 {
		t.Fatalf("linked material inventory: count=%d complete=%t err=%v", len(materials), complete, err)
	}
	material := materials[0]
	if material.Verified || material.Ownership != "" || material.Kind != "unknown" || material.Location == nil || material.Location.Reference != linked {
		t.Fatal("a protected hard link was adopted as destructively owned material")
	}
	encoded, err := json.Marshal(materials)
	if err != nil || strings.Contains(string(encoded), "foreign-recovery-material") || strings.Contains(string(encoded), foreign) {
		t.Fatal("inventory exposed foreign material or its outside path")
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "foreign-recovery-material" {
		t.Fatal("inspection changed foreign recovery material")
	}
}
