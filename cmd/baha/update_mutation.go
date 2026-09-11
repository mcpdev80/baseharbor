package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	maxReleaseArchiveBytes  = 128 << 20
	maxChecksumManifestSize = 4 << 20
	maxCandidateBinaryBytes = 128 << 20
)

var selfUpdateExecutable = os.Executable

func performSelfUpdate(ctx context.Context, check selfUpdateCheck, opts selfUpdateOptions, out, errOut io.Writer) error {
	if check.Relation == "up-to-date" {
		fmt.Fprintf(out, "BaseHarbor %s is already installed.\n", check.Target)
		return nil
	}
	if check.Relation == "target-older" {
		return errors.New("refusing to install an older BaseHarbor release through self-update")
	}
	if check.Relation == "development-build" && opts.Version == "" {
		return usageError("automatic self-update from a development build requires an explicit target version", "Use 'baha update --version VERSION' after reviewing the target release.")
	}
	if check.Relation != "update-available" && check.Relation != "development-build" {
		return fmt.Errorf("refusing self-update because installed/target version ordering is %q", check.Relation)
	}

	executable, err := selfUpdateExecutable()
	if err != nil {
		return fmt.Errorf("resolve running BaseHarbor executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("resolve absolute BaseHarbor executable path: %w", err)
	}
	if err := preflightSelfUpdateExecutable(executable); err != nil {
		return err
	}

	tempDir, err := os.MkdirTemp("", "baseharbor-self-update-*")
	if err != nil {
		return fmt.Errorf("create self-update staging directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return fmt.Errorf("protect self-update staging directory: %w", err)
	}

	candidate, err := downloadVerifiedSelfUpdateCandidate(ctx, check, tempDir)
	if err != nil {
		return err
	}
	if err := verifyCandidateVersion(ctx, candidate, check.Target); err != nil {
		return fmt.Errorf("verify downloaded BaseHarbor candidate: %w", err)
	}
	fmt.Fprintf(out, "[OK] release           %s downloaded and checksum verified\n", check.Target)
	fmt.Fprintf(out, "[OK] candidate         reports BaseHarbor %s\n", check.Target)

	_, controlPlaneExists := existingControlPlaneForSelfUpdate()
	_, localApplicationExists := localApplicationForSelfUpdate()

	recoveryPath, err := replaceExecutableWithRecovery(executable, candidate, check.Installed)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = restoreExecutableFromRecovery(executable, recoveryPath)
		}
	}()

	fmt.Fprintf(out, "[OK] cli               atomically replaced %s\n", executable)
	fmt.Fprintf(out, "Recovery binary: %s\n", recoveryPath)

	if err := verifyCandidateVersion(ctx, executable, check.Target); err != nil {
		return fmt.Errorf("post-update CLI verification failed; previous binary restored: %w", err)
	}
	if err := verifySelfUpdateRuntime(ctx, executable, check.Target, controlPlaneExists, localApplicationExists, out, errOut); err != nil {
		return fmt.Errorf("post-update runtime verification failed; previous CLI binary restored: %w", err)
	}

	rollback = false
	fmt.Fprintf(out, "BaseHarbor updated successfully: %s -> %s\n", displayInstalledVersion(check.Installed), check.Target)
	fmt.Fprintf(out, "Runtime image target: %s\n", defaultRuntimeImage(check.Target))
	return nil
}

func preflightSelfUpdateExecutable(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect running BaseHarbor executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("self-update refuses a symlinked BaseHarbor executable; replace the managed target through its installation method")
	}
	if !info.Mode().IsRegular() {
		return errors.New("running BaseHarbor executable is not a regular file")
	}
	dir := filepath.Dir(path)
	probe, err := os.CreateTemp(dir, ".baha-update-write-test-*")
	if err != nil {
		return fmt.Errorf("BaseHarbor executable directory %s is not writable; self-update never invokes sudo automatically: %w", dir, err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return fmt.Errorf("verify executable directory write access: %w", err)
	}
	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("clean executable directory write probe: %w", err)
	}
	return nil
}

func downloadVerifiedSelfUpdateCandidate(ctx context.Context, check selfUpdateCheck, dir string) (string, error) {
	checksums, err := downloadSelfUpdateBytes(ctx, check.ChecksumsURL, maxChecksumManifestSize)
	if err != nil {
		return "", fmt.Errorf("download release checksum manifest: %w", err)
	}
	expected, err := checksumForReleaseAsset(checksums, check.AssetName)
	if err != nil {
		return "", err
	}
	archive, err := downloadSelfUpdateBytes(ctx, check.AssetURL, maxReleaseArchiveBytes)
	if err != nil {
		return "", fmt.Errorf("download BaseHarbor release asset: %w", err)
	}
	actual := sha256.Sum256(archive)
	actualHex := hex.EncodeToString(actual[:])
	if !strings.EqualFold(expected, actualHex) {
		return "", fmt.Errorf("release archive checksum mismatch for %s", check.AssetName)
	}
	if check.AssetDigest != "" {
		algorithm, digest, ok := strings.Cut(check.AssetDigest, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(algorithm), "sha256") || !strings.EqualFold(strings.TrimSpace(digest), actualHex) {
			return "", fmt.Errorf("GitHub release asset digest mismatch for %s", check.AssetName)
		}
	}
	return extractBahaCandidate(archive, dir)
}

func downloadSelfUpdateBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("release download URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "baseharbor-baha/"+normalizeReleaseVersion(version))
	resp, err := releaseHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	reader := io.LimitReader(resp.Body, limit+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download exceeds %d byte safety limit", limit)
	}
	return data, nil
}

func checksumForReleaseAsset(manifest []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name != assetName {
			continue
		}
		digest := fields[0]
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size {
			return "", fmt.Errorf("invalid SHA-256 checksum for %s", assetName)
		}
		return strings.ToLower(digest), nil
	}
	return "", fmt.Errorf("checksums.txt does not contain %s", assetName)
}

func extractBahaCandidate(archive []byte, dir string) (string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", fmt.Errorf("open BaseHarbor release archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := false
	candidatePath := filepath.Join(dir, "baha")
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read BaseHarbor release archive: %w", err)
		}
		name := strings.TrimPrefix(filepath.ToSlash(header.Name), "./")
		if name != "baha" {
			continue
		}
		if found {
			return "", errors.New("release archive contains duplicate baha binary entries")
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return "", errors.New("release archive baha entry is not a regular file")
		}
		if header.Size <= 0 || header.Size > maxCandidateBinaryBytes {
			return "", fmt.Errorf("release archive baha binary has invalid size %d", header.Size)
		}
		file, err := os.OpenFile(candidatePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
		if err != nil {
			return "", fmt.Errorf("create staged BaseHarbor candidate: %w", err)
		}
		written, copyErr := io.CopyN(file, tr, header.Size)
		closeErr := file.Close()
		if copyErr != nil || written != header.Size {
			_ = os.Remove(candidatePath)
			return "", fmt.Errorf("extract BaseHarbor candidate: %w", copyErr)
		}
		if closeErr != nil {
			_ = os.Remove(candidatePath)
			return "", fmt.Errorf("close staged BaseHarbor candidate: %w", closeErr)
		}
		found = true
	}
	if !found {
		return "", errors.New("release archive does not contain baha binary")
	}
	return candidatePath, nil
}

func verifyCandidateVersion(ctx context.Context, path, target string) error {
	cmd := exec.CommandContext(ctx, path, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run candidate version command: %w", err)
	}
	line := strings.TrimSpace(string(output))
	prefix := "baha " + normalizeReleaseVersion(target) + " "
	if !strings.HasPrefix(line, prefix) {
		return fmt.Errorf("candidate reports unexpected version %q; expected %s", line, normalizeReleaseVersion(target))
	}
	return nil
}

func replaceExecutableWithRecovery(executable, candidate, installed string) (string, error) {
	currentInfo, err := os.Stat(executable)
	if err != nil {
		return "", fmt.Errorf("inspect current BaseHarbor binary: %w", err)
	}
	current, err := os.ReadFile(executable)
	if err != nil {
		return "", fmt.Errorf("read current BaseHarbor binary for recovery: %w", err)
	}
	candidateBytes, err := os.ReadFile(candidate)
	if err != nil {
		return "", fmt.Errorf("read staged BaseHarbor candidate: %w", err)
	}
	if len(candidateBytes) == 0 {
		return "", errors.New("staged BaseHarbor candidate is empty")
	}

	recoveryPath := executable + ".previous"
	if installed != "" && installed != "dev" {
		recoveryPath = executable + ".previous-" + safeVersionPathPart(installed)
	}
	if err := writeAtomicFile(recoveryPath, current, currentInfo.Mode().Perm()); err != nil {
		return "", fmt.Errorf("create BaseHarbor recovery binary: %w", err)
	}
	if err := writeAtomicFile(executable, candidateBytes, currentInfo.Mode().Perm()); err != nil {
		return "", fmt.Errorf("install BaseHarbor candidate; recovery binary retained at %s: %w", recoveryPath, err)
	}
	return recoveryPath, nil
}

func restoreExecutableFromRecovery(executable, recoveryPath string) error {
	info, err := os.Stat(recoveryPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(recoveryPath)
	if err != nil {
		return err
	}
	return writeAtomicFile(executable, data, info.Mode().Perm())
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".baha-atomic-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func verifySelfUpdateRuntime(ctx context.Context, executable, target string, controlPlaneExists, localApplicationExists bool, out, errOut io.Writer) error {
	env := selfUpdateEnvironment(target)
	if controlPlaneExists {
		if err := runUpdatedBaha(ctx, executable, env, out, errOut, "status"); err != nil {
			return fmt.Errorf("verify existing BaseHarbor control plane: %w", err)
		}
		fmt.Fprintln(out, "[OK] control-plane     status verified with updated CLI")
	}
	if localApplicationExists {
		if err := runUpdatedBaha(ctx, executable, env, out, errOut, "app", "apply"); err != nil {
			return fmt.Errorf("reconcile current application with updated runtime image %s: %w", defaultRuntimeImage(target), err)
		}
		fmt.Fprintf(out, "[OK] application       current repository reconciled with runtime image %s\n", defaultRuntimeImage(target))
	}
	return nil
}

func runUpdatedBaha(ctx context.Context, executable string, env []string, out, errOut io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = env
	cmd.Stdout = out
	cmd.Stderr = errOut
	return cmd.Run()
}

func selfUpdateEnvironment(target string) []string {
	key := "BASEHARBOR_RUNTIME_IMAGE="
	result := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, key) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, key+defaultRuntimeImage(target))
}

func existingControlPlaneForSelfUpdate() (bhruntime.Files, bool) {
	files, err := bhruntime.ExistingFiles("")
	return files, err == nil
}

func localApplicationForSelfUpdate() (string, bool) {
	manifest, err := application.FindRepositoryManifest(".")
	if err != nil {
		return "", false
	}
	return manifest, true
}

func safeVersionPathPart(value string) string {
	value = normalizeReleaseVersion(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

func displayInstalledVersion(value string) string {
	value = normalizeReleaseVersion(value)
	if value == "" {
		return "unknown"
	}
	return value
}
