package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const orasSetupVersion = "1.3.6"

var orasSetupHTTPClient = &http.Client{Timeout: 2 * time.Minute}

func ensureORASForCoreUp(ctx context.Context, in io.Reader, out io.Writer, assumeYes bool) error {
	return ensureORASForCoreUpWith(ctx, in, out, assumeYes, !noInput(ctx) && !assumeYes && readerIsTerminal(in), exec.LookPath, installORAS)
}

func ensureORASForCoreUpWith(ctx context.Context, in io.Reader, out io.Writer, assumeYes, interactive bool, lookPath func(string) (string, error), install func(context.Context) error) error {
	if _, err := lookPath("oras"); err == nil {
		return nil
	} else if !errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("check ORAS executable: %w", err)
	}
	fmt.Fprintln(out, "ORAS is missing. Baha uses it to resolve OCI image references to immutable digests and verify runtime artifacts; the container engine's mutable tag pull cannot provide that identity check.")
	if !interactive {
		return errors.New("ORAS is required before Core setup; install it manually or rerun 'baha up' in an interactive terminal to approve a user-local installation")
	}
	accepted, err := promptYesNo(orasPromptReader(in), out, "Install ORAS v"+orasSetupVersion+" for your user under ~/.local/bin?", false)
	if err != nil {
		return fmt.Errorf("ORAS installation was not approved: %w", err)
	}
	if !accepted {
		return errors.New("ORAS installation declined; Core setup was not started")
	}
	if err := install(ctx); err != nil {
		return fmt.Errorf("install ORAS: %w", err)
	}
	if _, err := lookPath("oras"); err != nil {
		return fmt.Errorf("ORAS installation completed but the executable is not on PATH: %w", err)
	}
	fmt.Fprintln(out, "ORAS is ready; continuing Core setup.")
	return nil
}

func installORAS(ctx context.Context) error {
	osName, arch, err := orasReleasePlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	archiveName := fmt.Sprintf("oras_%s_%s_%s.tar.gz", orasSetupVersion, osName, arch)
	baseURL := "https://github.com/oras-project/oras/releases/download/v" + orasSetupVersion + "/"
	archive, err := downloadORASAsset(ctx, baseURL+archiveName)
	if err != nil {
		return fmt.Errorf("download official ORAS release: %w", err)
	}
	checksums, err := downloadORASAsset(ctx, baseURL+"oras_"+orasSetupVersion+"_checksums.txt")
	if err != nil {
		return fmt.Errorf("download official ORAS checksums: %w", err)
	}
	want, err := orasChecksum(checksums, archiveName)
	if err != nil {
		return err
	}
	got := sha256.Sum256(archive)
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(got[:])), []byte(want)) != 1 {
		return errors.New("official ORAS release checksum did not match")
	}
	binary, err := orasBinaryFromArchive(archive)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find current user's home directory: %w", err)
	}
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return fmt.Errorf("create user-local bin directory: %w", err)
	}
	destination := filepath.Join(binDir, "oras")
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("refusing to replace existing %s; put that executable on PATH or move it aside first", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", destination, err)
	}
	tmp, err := os.CreateTemp(binDir, ".oras-install-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o700); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(binary); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, destination); err != nil {
		return fmt.Errorf("install user-local ORAS executable: %w", err)
	}
	return includeInstalledORASOnPath()
}

// includeInstalledORASOnPath lets this and later Baha invocations find a
// user-local installation without changing shell startup files.
func includeInstalledORASOnPath() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	binDir := filepath.Join(home, ".local", "bin")
	if _, err := os.Stat(filepath.Join(binDir, "oras")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	path := os.Getenv("PATH")
	if pathHasDirectory(path, binDir) {
		return nil
	}
	if path == "" {
		return os.Setenv("PATH", binDir)
	}
	return os.Setenv("PATH", path+string(os.PathListSeparator)+binDir)
}

func orasReleasePlatform(goos, goarch string) (string, string, error) {
	if goos != "linux" && goos != "darwin" {
		return "", "", fmt.Errorf("automatic ORAS installation does not support %s; install ORAS from https://oras.land/docs/installation/", goos)
	}
	if goarch != "amd64" && goarch != "arm64" {
		return "", "", fmt.Errorf("automatic ORAS installation does not support %s/%s; install ORAS from https://oras.land/docs/installation/", goos, goarch)
	}
	return goos, goarch, nil
}

func downloadORASAsset(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := orasSetupHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	const maxReleaseAsset = 64 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseAsset+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxReleaseAsset {
		return nil, errors.New("ORAS release asset exceeded the size limit")
	}
	return data, nil
}

func orasChecksum(checksums []byte, filename string) (string, error) {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			if len(fields[0]) != sha256.Size*2 {
				break
			}
			if _, err := hex.DecodeString(fields[0]); err == nil {
				return strings.ToLower(fields[0]), nil
			}
		}
	}
	return "", fmt.Errorf("official ORAS checksums do not contain a valid entry for %s", filename)
}

func orasBinaryFromArchive(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read ORAS release archive: %w", err)
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read ORAS release archive: %w", err)
		}
		if header.Name != "oras" || header.Typeflag != tar.TypeReg {
			continue
		}
		if header.Size <= 0 || header.Size > 64<<20 {
			return nil, errors.New("ORAS executable in release archive has an invalid size")
		}
		return io.ReadAll(io.LimitReader(tr, header.Size))
	}
	return nil, errors.New("ORAS executable is missing from the official release archive")
}

func pathHasDirectory(path, wanted string) bool {
	for _, entry := range filepath.SplitList(path) {
		if entry == wanted {
			return true
		}
	}
	return false
}

func orasPromptReader(in io.Reader) *bufio.Reader {
	if reader, ok := in.(*bufio.Reader); ok {
		return reader
	}
	return bufio.NewReader(in)
}
