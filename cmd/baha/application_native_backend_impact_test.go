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
        name string
        makeFile func(t *testing.T, path string)
    }{
        {"symlink", func(t *testing.T, path string) {
            outside := filepath.Join(t.TempDir(), "outside")
            if err := os.WriteFile(outside, []byte("foreign"), 0600); err != nil { t.Fatal(err) }
            if err := os.Symlink(outside, path); err != nil { t.Fatal(err) }
        }},
        {"group-writable", func(t *testing.T, path string) {
            if err := os.WriteFile(path, []byte("unprotected"), 0660); err != nil { t.Fatal(err) }
            if err := os.Chmod(path, 0660); err != nil { t.Fatal(err) }
        }},
        {"hardlink", func(t *testing.T, path string) {
            outside := filepath.Join(t.TempDir(), "outside")
            if err := os.WriteFile(outside, []byte("foreign"), 0600); err != nil { t.Fatal(err) }
            if err := os.Link(outside, path); err != nil { t.Fatal(err) }
        }},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            root := filepath.Join(t.TempDir(), "providers", "postgresql", "default")
            dir := filepath.Join(root, "runtime")
            if err := os.MkdirAll(dir, 0700); err != nil { t.Fatal(err) }
            path := filepath.Join(dir, "server-key.pem")
            tc.makeFile(t, path)
            materials, complete, err := observeOwnedProviderFiles("app", "target", "sql", root)
            if err != nil { t.Fatal(err) }
            if complete || len(materials) != 1 || materials[0].Verified || materials[0].Kind != "unknown" {
                t.Fatalf("observer must retain unsafe entry as incomplete: %+v complete=%t", materials, complete)
            }
            if sqlNativeFilesPermitCompleteInventory(complete, err) {
                t.Fatal("failed material verification must never authorize InventoryComplete")
            }
        })
    }
}

func TestNativeSQLInventoryKnownFilesRequireActualInspection(t *testing.T) {
    root := filepath.Join(t.TempDir(), "providers", "postgresql", "default")
    dir := filepath.Join(root, "runtime")
    if err := os.MkdirAll(dir, 0700); err != nil { t.Fatal(err) }
    path := filepath.Join(dir, "pg_hba.conf")
    if err := os.WriteFile(path, []byte("host all all 127.0.0.1/32 scram-sha-256\n"), 0600); err != nil { t.Fatal(err) }
    materials, complete, err := observeOwnedProviderFiles("app", "target", "sql", root)
    if err != nil || !complete || len(materials) != 1 || !materials[0].Verified {
        t.Fatalf("protected generated projection must remain eligible: %+v complete=%t err=%v", materials, complete, err)
    }
    if !sqlNativeFilesPermitCompleteInventory(complete, err) { t.Fatal("verified projection should permit later independent SQL probe") }

    // A recognized path containing invalid certificate material must stay
    // partial even when the path, owner and mode look expected.
    bad := filepath.Join(dir, "server-cert.pem")
    if err := os.WriteFile(bad, []byte("-----BEGIN CERTIFICATE-----\nYm9ndXM=\n-----END CERTIFICATE-----\n"), 0600); err != nil { t.Fatal(err) }
    materials, complete, err = observeOwnedProviderFiles("app", "target", "sql", root)
    if err != nil { t.Fatal(err) }
    if complete || sqlNativeFilesPermitCompleteInventory(complete, err) {
        t.Fatalf("failed certificate inspection authorized inventory: %+v", materials)
    }
}
