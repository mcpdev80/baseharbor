package main

import (
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestSQLNativeMaterialInventoryAllowsOnlyExpectedPublicProjection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "providers", "postgresql", "default")
	location := func(path string) *application.ResourceLocation {
		return &application.ResourceLocation{Kind: "path", Reference: path}
	}
	known := []application.ProviderMaterial{
		{Location: location(filepath.Join(root, "runtime", "pg_hba.conf"))},
		{Location: location(filepath.Join(root, "runtime", "server-cert.pem"))},
		{Location: location(filepath.Join(root, "runtime", "server-key.pem"))},
		{Location: location(filepath.Join(root, "service-access", "pki", "ca.pem"))},
		{Location: location(filepath.Join(root, "service-access", "pki", "server-cert.pem"))},
		{Verified: true, Location: location(filepath.Join(root, "service-access", "pki", "server-key.pem"))},
	}
	if !sqlNativeMaterialInventoryAllowsPublicProjection(known, root) {
		t.Fatal("known public SQL runtime projections should not block empty-data inventory")
	}
	withUnknown := append([]application.ProviderMaterial(nil), known...)
	withUnknown = append(withUnknown, application.ProviderMaterial{Location: location(filepath.Join(root, "unexpected.db"))})
	if sqlNativeMaterialInventoryAllowsPublicProjection(withUnknown, root) {
		t.Fatal("unexpected unverified material must keep SQL inventory partial")
	}
	withoutLocation := append([]application.ProviderMaterial(nil), known...)
	withoutLocation = append(withoutLocation, application.ProviderMaterial{})
	if sqlNativeMaterialInventoryAllowsPublicProjection(withoutLocation, root) {
		t.Fatal("unlocated unverified material must keep SQL inventory partial")
	}
}
