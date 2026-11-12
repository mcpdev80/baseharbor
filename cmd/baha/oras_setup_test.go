package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureORASForCoreUpSkipsPromptWhenInstalled(t *testing.T) {
	installed := false
	installCalls := 0
	err := ensureORASForCoreUpWith(context.Background(), strings.NewReader(""), io.Discard, false, true,
		func(string) (string, error) { return "/usr/bin/oras", nil },
		func(context.Context) error { installCalls++; installed = true; return nil })
	if err != nil || installed || installCalls != 0 {
		t.Fatalf("ensure ORAS: err=%v installed=%t calls=%d", err, installed, installCalls)
	}
}

func TestEnsureORASForCoreUpRequiresExplicitInteractiveConsent(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input       string
		interactive bool
		wantInstall bool
	}{
		{name: "declined", input: "n\n", interactive: true},
		{name: "accepted", input: "y\n", interactive: true, wantInstall: true},
		{name: "noninteractive", input: "y\n", interactive: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			installed := false
			var output bytes.Buffer
			lookup := func(string) (string, error) {
				if installed {
					return "/home/test/.local/bin/oras", nil
				}
				return "", &exec.Error{Name: "oras", Err: exec.ErrNotFound}
			}
			err := ensureORASForCoreUpWith(context.Background(), bufio.NewReader(strings.NewReader(tc.input)), &output, false, tc.interactive, lookup,
				func(context.Context) error { installed = true; return nil })
			if tc.wantInstall {
				if err != nil || !installed || !strings.Contains(output.String(), "Install ORAS") {
					t.Fatalf("accepted install: err=%v installed=%t output=%q", err, installed, output.String())
				}
				return
			}
			if err == nil || installed {
				t.Fatalf("missing consent should fail without installing: err=%v installed=%t", err, installed)
			}
		})
	}
}

func TestEnsureORASForCoreUpPropagatesInstallFailure(t *testing.T) {
	want := errors.New("network unavailable")
	err := ensureORASForCoreUpWith(context.Background(), strings.NewReader("y\n"), io.Discard, false, true,
		func(string) (string, error) { return "", exec.ErrNotFound },
		func(context.Context) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("install error = %v, want %v", err, want)
	}
}

func TestRuntimeUpRequiresORASBeforeCoreBootstrap(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	var output bytes.Buffer
	err := runtimeUpCommand(context.Background(), []string{"--yes"}, &output, &output)
	if err == nil || !strings.Contains(err.Error(), "ORAS is required before Core setup") {
		t.Fatalf("baha up without ORAS error = %v", err)
	}
	if !strings.Contains(output.String(), "resolve OCI image references to immutable digests") {
		t.Fatalf("missing ORAS explanation: %q", output.String())
	}
}

func TestORASReleasePlatformAndChecksum(t *testing.T) {
	if osName, arch, err := orasReleasePlatform("linux", "arm64"); err != nil || osName != "linux" || arch != "arm64" {
		t.Fatalf("platform = %s/%s, %v", osName, arch, err)
	}
	if _, _, err := orasReleasePlatform("windows", "amd64"); err == nil {
		t.Fatal("unsupported platform accepted")
	}
	const name = "oras_1.3.6_linux_amd64.tar.gz"
	want := strings.Repeat("a", 64)
	got, err := orasChecksum([]byte(want+"  "+name+"\n"), name)
	if err != nil || got != want {
		t.Fatalf("checksum = %q, %v", got, err)
	}
	if _, err := orasChecksum([]byte(want+"  other.tar.gz\n"), name); err == nil {
		t.Fatal("missing checksum accepted")
	}
}

func TestORASBinaryFromArchiveExtractsOnlyRegularBinary(t *testing.T) {
	archive := orasTestArchive(t, "oras", []byte("binary-content"), tar.TypeReg)
	got, err := orasBinaryFromArchive(archive)
	if err != nil || string(got) != "binary-content" {
		t.Fatalf("extract = %q, %v", got, err)
	}
	archive = orasTestArchive(t, "../oras", []byte("bad"), tar.TypeSymlink)
	if _, err := orasBinaryFromArchive(archive); err == nil {
		t.Fatal("archive without a regular executable accepted")
	}
}

func TestIncludeInstalledORASOnPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "oras"), []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	original := t.TempDir()
	t.Setenv("PATH", original)
	if err := includeInstalledORASOnPath(); err != nil {
		t.Fatal(err)
	}
	if got := filepath.SplitList(os.Getenv("PATH")); len(got) != 2 || got[0] != original || got[1] != binDir {
		t.Fatalf("PATH = %v, want system path followed by %s", got, binDir)
	}
}

func orasTestArchive(t *testing.T, name string, data []byte, kind byte) []byte {
	t.Helper()
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	tw := tar.NewWriter(zw)
	header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: kind}
	if kind == tar.TypeSymlink {
		header.Linkname = "/tmp/evil"
		header.Size = 0
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
