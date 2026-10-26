package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A material name is not proof of safe ownership. These cases exercise the
// production observer and the exact file-completeness condition used before
// a native SQL consumer inventory may set InventoryComplete.
func TestNativeSQLInventoryRejectsUnverifiedMaterialRegardlessOfKnownPath(t *testing.T) {
	cases := []struct {
		name     string
		makeFile func(t *testing.T, path string)
	}{
		{"symlink", func(t *testing.T, path string) {
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
		}},
		{"group-writable", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("unprotected"), 0660); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0660); err != nil {
				t.Fatal(err)
			}
		}},
		{"hardlink", func(t *testing.T, path string) {
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("foreign"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(outside, path); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "providers", "postgresql", "default")
			dir := filepath.Join(root, "runtime")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "server-key.pem")
			tc.makeFile(t, path)
			materials, complete, err := observeOwnedProviderFiles("app", "target", "sql", root)
			if err != nil {
				t.Fatal(err)
			}
			if complete || len(materials) != 1 || materials[0].Verified || materials[0].Kind != "unknown" {
				t.Fatalf("observer must retain unsafe entry as incomplete: %+v complete=%t", materials, complete)
			}
			if sqlNativeMaterialInventoryPermitsSQLProbe(materials, root, complete, err) {
				t.Fatal("failed material verification must never authorize InventoryComplete")
			}
		})
	}
}

func TestNativeSQLInventoryKnownFilesRequireActualInspection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "providers", "postgresql", "default")
	dir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pg_hba.conf")
	if err := os.WriteFile(path, []byte("host all all 127.0.0.1/32 scram-sha-256\n"), 0600); err != nil {
		t.Fatal(err)
	}
	materials, complete, err := observeOwnedProviderFiles("app", "target", "sql", root)
	if err != nil || !complete || len(materials) != 1 || !materials[0].Verified {
		t.Fatalf("protected generated projection must remain eligible: %+v complete=%t err=%v", materials, complete, err)
	}
	if !sqlNativeMaterialInventoryPermitsSQLProbe(materials, root, complete, err) {
		t.Fatal("verified projection should permit later independent SQL probe")
	}

	// A recognized path containing invalid certificate material must stay
	// partial even when the path, owner and mode look expected.
	bad := filepath.Join(dir, "server-cert.pem")
	if err := os.WriteFile(bad, []byte("-----BEGIN CERTIFICATE-----\nYm9ndXM=\n-----END CERTIFICATE-----\n"), 0600); err != nil {
		t.Fatal(err)
	}
	materials, complete, err = observeOwnedProviderFiles("app", "target", "sql", root)
	if err != nil {
		t.Fatal(err)
	}
	if complete || sqlNativeMaterialInventoryPermitsSQLProbe(materials, root, complete, err) {
		t.Fatalf("failed certificate inspection authorized inventory: %+v", materials)
	}
}

func TestNativeSQLGeneratedProjectionRequiresProtectedAncestorAndSource(t *testing.T) {
	root := filepath.Join(t.TempDir(), "providers", "postgresql", "default")
	dir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pg_hba.conf")
	generated := "local all all trust\nhostssl all all 0.0.0.0/0 scram-sha-256\nhostssl all all ::/0 scram-sha-256\nhostnossl all all 0.0.0.0/0 reject\nhostnossl all all ::/0 reject\n"
	if err := os.WriteFile(path, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	check := func() bool {
		materials, complete, err := observeOwnedProviderFiles("app", "target", "sql", root)
		return sqlNativeMaterialInventoryPermitsSQLProbe(materials, root, complete, err)
	}
	if !check() {
		t.Fatal("known genuinely generated public configuration should remain eligible for independent SQL probe")
	}
	if err := os.Chmod(path, 0664); err != nil {
		t.Fatal(err)
	}
	if check() {
		t.Fatal("group-writable generated projection bypassed protection")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("different content"), 0644); err != nil {
		t.Fatal(err)
	}
	if check() {
		t.Fatal("unexpected content on generated path bypassed provenance")
	}
	if err := os.WriteFile(path, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if check() {
		t.Fatal("unprotected parent directory bypassed ownership")
	}
}

func TestNativeSQLProductionMaterialGateRejectsOversizedAndBrokenPublicSources(t *testing.T) {
	root := filepath.Join(t.TempDir(), "providers", "postgresql", "default")
	runtimeDir := filepath.Join(root, "runtime")
	pki := filepath.Join(root, "service-access", "pki")
	for _, dir := range []string{runtimeDir, pki} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	check := func() bool {
		materials, complete, err := observeOwnedProviderFiles("app", "target", "sql", root)
		return sqlNativeMaterialInventoryPermitsSQLProbe(materials, root, complete, err)
	}
	cert := []byte("-----BEGIN CERTIFICATE-----\nYm9ndXM=\n-----END CERTIFICATE-----\n")
	for _, path := range []string{filepath.Join(runtimeDir, "server-cert.pem"), filepath.Join(pki, "ca.pem")} {
		if err := os.WriteFile(path, cert, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if check() {
			t.Fatalf("invalid public certificate accepted: %s", path)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(runtimeDir, "server-key.pem")
	if err := os.WriteFile(key, []byte("-----BEGIN PRIVATE KEY-----\na2V5\n-----END PRIVATE KEY-----\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if check() {
		t.Fatal("unproven runtime key accepted without protected source")
	}
	source := filepath.Join(pki, "server-key.pem")
	if err := os.WriteFile(source, []byte("-----BEGIN PRIVATE KEY-----\na2V5\n-----END PRIVATE KEY-----\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(source, 0644); err != nil {
		t.Fatal(err)
	}
	if check() {
		t.Fatal("insecure source key bypassed private-key authority")
	}
	if err := os.Remove(key); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}

	hba := filepath.Join(runtimeDir, "pg_hba.conf")
	generated := "local all all trust\nhostssl all all 0.0.0.0/0 scram-sha-256\nhostssl all all ::/0 scram-sha-256\nhostnossl all all 0.0.0.0/0 reject\nhostnossl all all ::/0 reject\n"
	if err := os.WriteFile(hba, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(hba, 0644); err != nil {
		t.Fatal(err)
	}
	if !check() {
		t.Fatal("expected public projection rejected")
	}
	if err := os.WriteFile(hba, make([]byte, sqlProjectionMaxBytes+1), 0644); err != nil {
		t.Fatal(err)
	}
	if check() {
		t.Fatal("oversized projection bypassed bound")
	}
	info, err := os.Lstat(hba)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlReadBoundedProjection(hba, info, sqlPublicProjectionOwner); err == nil {
		t.Fatal("oversized read was not bounded by file size")
	}
	if err := os.WriteFile(hba, []byte(generated), 0644); err != nil {
		t.Fatal(err)
	}
	info, err = os.Lstat(hba)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := os.OpenFile(hba, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Truncate(sqlProjectionMaxBytes + 1); err != nil {
		t.Fatal(err)
	}
	handle.Close()
	if _, err := sqlReadBoundedProjection(hba, info, sqlPublicProjectionOwner); err == nil {
		t.Fatal("growth after size inspection bypassed read bound")
	}
}
