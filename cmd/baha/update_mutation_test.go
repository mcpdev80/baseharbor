package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChecksumForReleaseAsset(t *testing.T) {
	digest := strings.Repeat("a", 64)
	manifest := []byte(digest + "  baseharbor_linux_amd64.tar.gz\n")
	got, err := checksumForReleaseAsset(manifest, "baseharbor_linux_amd64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != digest {
		t.Fatalf("unexpected checksum: %s", got)
	}
	if _, err := checksumForReleaseAsset(manifest, "missing.tar.gz"); err == nil {
		t.Fatal("expected missing asset checksum error")
	}
}

func TestExtractBahaCandidateRejectsNonRegularEntry(t *testing.T) {
	archive := buildSelfUpdateArchive(t, tar.TypeSymlink, []byte("ignored"))
	if _, err := extractBahaCandidate(archive, t.TempDir()); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected regular-file rejection, got %v", err)
	}
}

func TestExtractBahaCandidateWritesExecutable(t *testing.T) {
	payload := []byte("candidate-binary")
	archive := buildSelfUpdateArchive(t, tar.TypeReg, payload)
	path, err := extractBahaCandidate(archive, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("candidate mismatch: %q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("candidate is not executable: %o", info.Mode().Perm())
	}
}

func TestReplaceExecutableWithRecoveryAndRestore(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "baha")
	candidate := filepath.Join(dir, "candidate")
	oldBytes := []byte("old-binary")
	newBytes := []byte("new-binary")
	if err := os.WriteFile(executable, oldBytes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(candidate, newBytes, 0o700); err != nil {
		t.Fatal(err)
	}

	recovery, err := replaceExecutableWithRecovery(executable, candidate, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBytes) {
		t.Fatalf("installed binary mismatch: %q", got)
	}
	recovered, err := os.ReadFile(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recovered, oldBytes) {
		t.Fatalf("recovery binary mismatch: %q", recovered)
	}

	if err := restoreExecutableFromRecovery(executable, recovery); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldBytes) {
		t.Fatalf("restored binary mismatch: %q", got)
	}
}

func TestPreflightSelfUpdateExecutableRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real-baha")
	link := filepath.Join(dir, "baha")
	if err := os.WriteFile(target, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := preflightSelfUpdateExecutable(link); err == nil || !strings.Contains(err.Error(), "symlinked") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestReleaseArchiveChecksumRoundTrip(t *testing.T) {
	archive := buildSelfUpdateArchive(t, tar.TypeReg, []byte("baha"))
	sum := sha256.Sum256(archive)
	manifest := []byte(hex.EncodeToString(sum[:]) + "  release.tar.gz\n")
	got, err := checksumForReleaseAsset(manifest, "release.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if got != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected checksum: %s", got)
	}
}

func TestPerformSelfUpdateReplacesBinaryAndRetainsRecovery(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "baha")
	oldBinary := []byte("#!/bin/sh\necho 'baha 0.2.0 (commit old, built test)'\n")
	newBinary := []byte("#!/bin/sh\necho 'baha 0.3.0 (commit new, built test)'\n")
	if err := os.WriteFile(executable, oldBinary, 0o755); err != nil {
		t.Fatal(err)
	}

	archive := buildSelfUpdateArchive(t, tar.TypeReg, newBinary)
	check, server := selfUpdateFixture(t, archive)
	defer server.Close()
	withSelfUpdateMutationFixture(t, executable, server)

	var out strings.Builder
	if err := performSelfUpdate(context.Background(), check, selfUpdateOptions{Yes: true, Version: "0.3.0"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, newBinary) {
		t.Fatalf("installed binary mismatch: %q", installed)
	}
	recovery, err := os.ReadFile(executable + ".previous-0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(recovery, oldBinary) {
		t.Fatalf("recovery binary mismatch: %q", recovery)
	}
	if !strings.Contains(out.String(), "BaseHarbor updated successfully: 0.2.0 -> 0.3.0") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestPerformSelfUpdateRestoresBinaryWhenRuntimeVerificationFails(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "baha")
	oldBinary := []byte("#!/bin/sh\necho 'baha 0.2.0 (commit old, built test)'\n")
	newBinary := []byte("#!/bin/sh\nif [ \"$1\" = \"version\" ]; then echo 'baha 0.3.0 (commit new, built test)'; exit 0; fi\nexit 42\n")
	if err := os.WriteFile(executable, oldBinary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "baseharbor.yaml"), []byte("name: demo\nenvironment: dev\nservices: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	archive := buildSelfUpdateArchive(t, tar.TypeReg, newBinary)
	check, server := selfUpdateFixture(t, archive)
	defer server.Close()
	withSelfUpdateMutationFixture(t, executable, server)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	var out strings.Builder
	err = performSelfUpdate(context.Background(), check, selfUpdateOptions{Yes: true, Version: "0.3.0"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "previous CLI binary restored") {
		t.Fatalf("expected runtime verification rollback error, got %v", err)
	}
	installed, readErr := os.ReadFile(executable)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(installed, oldBinary) {
		t.Fatalf("old binary was not restored: %q", installed)
	}
}

func selfUpdateFixture(t *testing.T, archive []byte) (selfUpdateCheck, *httptest.Server) {
	t.Helper()
	assetName := "baseharbor_linux_amd64.tar.gz"
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	manifest := []byte(digest + "  " + assetName + "\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/checksums.txt":
			_, _ = w.Write(manifest)
		case "/asset.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	return selfUpdateCheck{
		Installed:    "0.2.0",
		Target:       "0.3.0",
		Relation:     "update-available",
		AssetName:    assetName,
		AssetURL:     server.URL + "/asset.tar.gz",
		AssetDigest:  "sha256:" + digest,
		ChecksumsURL: server.URL + "/checksums.txt",
	}, server
}

func withSelfUpdateMutationFixture(t *testing.T, executable string, server *httptest.Server) {
	t.Helper()
	oldExecutable := selfUpdateExecutable
	oldClient := releaseHTTPClient
	selfUpdateExecutable = func() (string, error) { return executable, nil }
	releaseHTTPClient = server.Client()
	t.Setenv("BASEHARBOR_STATE_DIR", filepath.Join(t.TempDir(), "runtime-not-created"))
	t.Cleanup(func() {
		selfUpdateExecutable = oldExecutable
		releaseHTTPClient = oldClient
	})
}

func buildSelfUpdateArchive(t *testing.T, typeflag byte, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	header := &tar.Header{
		Name:     "baha",
		Mode:     0o755,
		Size:     int64(len(payload)),
		Typeflag: typeflag,
	}
	if typeflag == tar.TypeSymlink {
		header.Size = 0
		header.Linkname = "somewhere"
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if typeflag == tar.TypeReg {
		if _, err := io.Copy(tw, bytes.NewReader(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
