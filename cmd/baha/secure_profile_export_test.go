package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
)

// TestSecureProfileExportRejectsMissingPasswordFile proves the export refuses to
// run without an owner-only password file (fail-closed on the secret input).
func TestSecureProfileExportRejectsMissingPasswordFile(t *testing.T) {
	_, _, _, err := parseSecureProfileExportArgs([]string{})
	if err == nil || !strings.Contains(err.Error(), "--password-file is required") {
		t.Fatalf("parseSecureProfileExportArgs() error = %v, want --password-file required", err)
	}
}

// TestSecureProfileExportRejectsArgPassword proves the export never accepts the
// password through argv (only --password-file).
func TestSecureProfileExportRejectsArgPassword(t *testing.T) {
	_, _, _, err := parseSecureProfileExportArgs([]string{"--password-file"})
	if err == nil || !strings.Contains(err.Error(), "--password-file requires a file path") {
		t.Fatalf("parseSecureProfileExportArgs([--password-file]) error = %v, want path required", err)
	}
}

// TestSecureProfileExportPasswordFilePerms proves the password file must be
// owner-only (no group/other access), matching the backup password-file guard.
func TestSecureProfileExportPasswordFilePerms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(path, []byte("correct horse battery staple"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecureProfileExportPassword(path); err == nil || !strings.Contains(err.Error(), "accessible by group or others") {
		t.Fatalf("readSecureProfileExportPassword(perms 0644) error = %v, want perms rejection", err)
	}
}

// TestSecureProfileExportPasswordTooShort proves a short password fails closed.
func TestSecureProfileExportPasswordTooShort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSecureProfileExportPassword(path); err == nil || !strings.Contains(err.Error(), "at least 12 bytes") {
		t.Fatalf("readSecureProfileExportPassword(short) error = %v, want min-length rejection", err)
	}
}

// TestSecureProfileExportWritesEncryptedFile is the production-path e2e cell:
// write the export through the real command's write seam, then assert (a) the
// ciphertext with the correct password decrypts back to the secret, and (b)
// the ciphertext with no/wrong password fails closed. A removed encryption
// binding fails (a) (secret never round-trips) and a tolerant decryption fails
// (b) — both are the RED probes this cell carries.
func TestSecureProfileExportWritesEncryptedFile(t *testing.T) {
	password := []byte("correct horse battery staple")
	manifest := applicationbackup.ExportManifest{
		ApplicationID: "11111111-1111-4111-8111-111111111111",
		Application:   "mailflow",
		Environment:   "dev",
	}
	secret := []byte("SECRET-PLAINTEXT-MARKER-4fIDBAUG")
	entries := []applicationbackup.SecretEntry{
		{Name: "developer-access/password", Data: secret},
	}
	envelope, err := applicationbackup.BuildExport(manifest, entries, password)
	if err != nil {
		t.Fatalf("BuildExport() error = %v", err)
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "export.bhexport")
	if err := writeSecureProfileExport(out, envelope); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// Correct password round-trips the secret: a removed encryption binding would
	// make this fail (no plaintext to decrypt).
	manifest2, entries2, err := applicationbackup.OpenExport(data, password)
	if err != nil {
		t.Fatalf("OpenExport(correct password) error = %v", err)
	}
	if manifest2.Application != "mailflow" || len(entries2) != 1 || string(entries2[0].Data) != string(secret) {
		t.Fatalf("round-trip secret mismatch: %#v, %+v", manifest2, entries2)
	}
	// No/wrong password fails closed: a tolerant decryption fails this.
	if _, _, err := applicationbackup.OpenExport(data, nil); err == nil {
		t.Fatal("OpenExport(nil) = nil error; ciphertext readable without password (must be RED)")
	}
	if _, _, err := applicationbackup.OpenExport(data, []byte("wrong")); err == nil {
		t.Fatal("OpenExport(wrong password) = nil error; ciphertext opens without password (must be RED)")
	}
}

// TestSecureProfileExportWrongPasswordFailsClosed is the counter-probe: the same
// ciphertext that opens with the correct password must fail closed on a wrong
// password (never a half state). It also is the tolerant-mutant RED probe: a
// decryption that accepts a wrong password fails this cell.
func TestSecureProfileExportWrongPasswordFailsClosed(t *testing.T) {
	password := []byte("correct password here")
	manifest := applicationbackup.ExportManifest{ApplicationID: "1", Application: "mailflow", Environment: "dev"}
	entries := []applicationbackup.SecretEntry{{Name: "developer-access/password", Data: []byte("classified")}}
	envelope, err := applicationbackup.BuildExport(manifest, entries, password)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := applicationbackup.OpenExport(envelope, []byte("wrong password")); err == nil {
		t.Fatal("MUTANT REPRO: OpenExport(wrong password) succeeded — export is tolerant, must be RED")
	}
}
