package hosttrust

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const stateVersion = 1

type AnchorRecord struct {
	Fingerprint     string    `json:"fingerprint"`
	IssuerReference string    `json:"issuer_reference,omitempty"`
	Backend         string    `json:"backend"`
	Path            string    `json:"path"`
	InstalledAt     time.Time `json:"installed_at"`
}

type State struct {
	Version int            `json:"version"`
	Anchors []AnchorRecord `json:"anchors,omitempty"`
}

type Status struct {
	Fingerprint string `json:"fingerprint"`
	Trusted     bool   `json:"trusted"`
	Owned       bool   `json:"owned"`
	Backend     string `json:"backend,omitempty"`
	Path        string `json:"path,omitempty"`
}

type Backend interface {
	Name() string
	AnchorPath(fingerprint string) (string, error)
	Install(context.Context, string, string) error
	Remove(context.Context, string) error
}

func ParseCA(pemData []byte) (*x509.Certificate, string, error) {
	block, _ := pem.Decode(pemData)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, "", errors.New("managed trust bundle does not contain an X.509 certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse managed trust certificate: %w", err)
	}
	if !cert.IsCA {
		return nil, "", errors.New("managed trust certificate is not a CA")
	}
	sum := sha256.Sum256(cert.Raw)
	return cert, hex.EncodeToString(sum[:]), nil
}

func Export(path string, pemData []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("CA export path is required")
	}
	if _, _, err := ParseCA(pemData); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("refusing to overwrite existing CA export %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, pemData, 0o644); err != nil {
		return err
	}
	return os.Chmod(path, 0o644)
}

func Inspect(stateDir string, pemData []byte) (Status, error) {
	cert, fingerprint, err := ParseCA(pemData)
	if err != nil {
		return Status{}, err
	}
	trusted, err := systemTrusted(cert)
	if err != nil {
		return Status{}, err
	}
	state, err := loadState(stateDir)
	if err != nil {
		return Status{}, err
	}
	status := Status{Fingerprint: fingerprint, Trusted: trusted}
	for _, record := range state.Anchors {
		if record.Fingerprint == fingerprint {
			status.Owned = true
			status.Backend = record.Backend
			status.Path = record.Path
			break
		}
	}
	return status, nil
}

func Install(ctx context.Context, stateDir string, pemData []byte, issuerReference string, backend Backend) (Status, error) {
	cert, fingerprint, err := ParseCA(pemData)
	if err != nil {
		return Status{}, err
	}
	trusted, err := systemTrusted(cert)
	if err != nil {
		return Status{}, err
	}
	state, err := loadState(stateDir)
	if err != nil {
		return Status{}, err
	}

	var owned *AnchorRecord
	for index := range state.Anchors {
		if state.Anchors[index].Fingerprint != fingerprint {
			continue
		}
		owned = &state.Anchors[index]
		if trusted {
			return Status{Fingerprint: fingerprint, Trusted: true, Owned: true, Backend: owned.Backend, Path: owned.Path}, nil
		}
		if err := verifyRecordedAnchor(*owned); err != nil {
			return Status{}, err
		}
		break
	}
	if trusted {
		// Do not claim ownership of trust installed by an operator or another tool.
		return Status{Fingerprint: fingerprint, Trusted: true}, nil
	}

	if backend == nil {
		if owned != nil {
			backend, err = resolveBackend(owned.Backend)
		} else {
			backend, err = DetectSystemBackend()
		}
		if err != nil {
			return Status{}, err
		}
	}

	path, err := backend.AnchorPath(fingerprint)
	if err != nil {
		return Status{}, err
	}
	if owned != nil {
		if backend.Name() != owned.Backend {
			return Status{}, fmt.Errorf("recorded host trust backend %q does not match selected backend %q", owned.Backend, backend.Name())
		}
		path = owned.Path
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return Status{}, err
	}
	tmp, err := os.CreateTemp(stateDir, "managed-ca-*.pem")
	if err != nil {
		return Status{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return Status{}, err
	}
	if _, err := tmp.Write(pemData); err != nil {
		_ = tmp.Close()
		return Status{}, err
	}
	if err := tmp.Close(); err != nil {
		return Status{}, err
	}
	if err := backend.Install(ctx, tmpPath, path); err != nil {
		return Status{}, err
	}

	now := time.Now().UTC()
	if owned == nil {
		state.Anchors = append(state.Anchors, AnchorRecord{
			Fingerprint: fingerprint, IssuerReference: strings.TrimSpace(issuerReference),
			Backend: backend.Name(), Path: path, InstalledAt: now,
		})
	} else {
		owned.IssuerReference = strings.TrimSpace(issuerReference)
		owned.InstalledAt = now
	}
	if err := saveState(stateDir, state); err != nil {
		// Trust was installed but ownership persistence failed. Roll it back
		// immediately so BaseHarbor never leaves untracked owned trust behind.
		_ = backend.Remove(ctx, path)
		return Status{}, fmt.Errorf("persist host trust ownership: %w", err)
	}
	return Status{Fingerprint: fingerprint, Trusted: true, Owned: true, Backend: backend.Name(), Path: path}, nil
}

// RemovalResult retains explicit evidence for every owned, preserved or refused CA.
type RemovalResult struct {
	Removed   []AnchorRecord `json:"removed,omitempty"`
	Preserved []AnchorRecord `json:"preserved,omitempty"`
	Untracked []string       `json:"preserved_untracked,omitempty"`
}

func RemoveOwnedDetailed(ctx context.Context, stateDir string) (RemovalResult, error) {
	state, err := loadState(stateDir)
	if err != nil {
		return RemovalResult{}, err
	}
	result := RemovalResult{}
	var failures error
	retained := make([]AnchorRecord, 0, len(state.Anchors))
	for _, record := range state.Anchors {
		backend, resolveErr := resolveBackend(record.Backend)
		if resolveErr == nil {
			var expected string
			expected, resolveErr = backend.AnchorPath(record.Fingerprint)
			if resolveErr == nil && filepath.Clean(expected) != filepath.Clean(record.Path) {
				resolveErr = fmt.Errorf("recorded anchor path does not match fingerprint %s", record.Fingerprint)
			}
		}
		if resolveErr == nil {
			resolveErr = verifyRecordedAnchor(record)
		}
		if resolveErr == nil {
			resolveErr = backend.Remove(ctx, record.Path)
		}
		if resolveErr != nil {
			failures = errors.Join(failures, fmt.Errorf("PRESERVED %s (%s): %w", record.Path, record.Fingerprint, resolveErr))
			retained = append(retained, record)
			result.Preserved = append(result.Preserved, record)
			continue
		}
		result.Removed = append(result.Removed, record)

	}
	if len(retained) > 0 {
		state.Anchors = retained
		if err := saveState(stateDir, state); err != nil {
			return result, errors.Join(failures, err)
		}
	} else if len(state.Anchors) > 0 || len(result.Removed) > 0 {
		if err := os.Remove(statePath(stateDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, errors.Join(failures, err)
		}
	}
	return result, failures
}

func RemoveOwned(ctx context.Context, stateDir string) (int, error) {
	result, err := RemoveOwnedDetailed(ctx, stateDir)
	return len(result.Removed), err
}

func verifyRecordedAnchor(record AnchorRecord) error {
	info, statErr := os.Lstat(record.Path)
	if errors.Is(statErr, os.ErrNotExist) {
		return nil
	}
	if statErr != nil {
		return statErr
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("PRESERVED host trust anchor %s is not a regular file", record.Path)
	}
	data, err := os.ReadFile(record.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect recorded host trust anchor %s: %w", record.Path, err)
	}
	_, fingerprint, err := ParseCA(data)
	if err != nil {
		return fmt.Errorf("validate recorded host trust anchor %s: %w", record.Path, err)
	}
	if fingerprint != record.Fingerprint {
		return fmt.Errorf("refusing to remove host trust anchor %s: installed certificate fingerprint no longer matches BaseHarbor ownership state", record.Path)
	}
	return nil
}

func StateRecords(stateDir string) ([]AnchorRecord, error) {
	state, err := loadState(stateDir)
	if err != nil {
		return nil, err
	}
	return append([]AnchorRecord(nil), state.Anchors...), nil
}

// UntrackedCandidates reports BaseHarbor-named anchors which lack verified
// ownership evidence. Their names do NOT imply ownership and they are never
// eligible for automatic deletion.
func UntrackedCandidates(stateDir string) ([]string, error) {
	directories := []struct{ path, extension string }{
		{"/usr/local/share/ca-certificates", ".crt"},
		{"/etc/pki/ca-trust/source/anchors", ".pem"},
	}
	return findUntrackedCandidates(stateDir, directories)
}

func findUntrackedCandidates(stateDir string, directories []struct{ path, extension string }) ([]string, error) {
	state, err := loadState(stateDir)
	if err != nil {
		return nil, err
	}
	owned := map[string]bool{}
	for _, record := range state.Anchors {
		owned[filepath.Clean(record.Path)] = true
	}
	candidates := []string{}
	for _, dir := range directories {
		entries, err := os.ReadDir(dir.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect host trust candidates: %w", err)
		}
		for _, entry := range entries {
			file := entry.Name()
			if !strings.HasPrefix(file, "baseharbor-") || !strings.HasSuffix(file, dir.extension) {
				continue
			}
			fingerprintPart := strings.TrimSuffix(strings.TrimPrefix(file, "baseharbor-"), dir.extension)
			if len(fingerprintPart) != 16 {
				continue
			}
			valid := true
			for _, r := range fingerprintPart {
				if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			path := filepath.Join(dir.path, file)
			if !owned[filepath.Clean(path)] {
				candidates = append(candidates, path)
			}
		}
	}
	return candidates, nil
}

func statePath(stateDir string) string {
	return filepath.Join(stateDir, "host-trust.json")
}

func loadState(stateDir string) (State, error) {
	data, err := os.ReadFile(statePath(stateDir))
	if errors.Is(err, os.ErrNotExist) {
		return State{Version: stateVersion}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode host trust ownership state: %w", err)
	}
	if state.Version != stateVersion {
		return State{}, fmt.Errorf("unsupported host trust state version %d", state.Version)
	}
	return state, nil
}

func saveState(stateDir string, state State) error {
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	state.Version = stateVersion
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := statePath(stateDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func systemTrusted(cert *x509.Certificate) (bool, error) {
	roots, err := x509.SystemCertPool()
	if err != nil {
		return false, fmt.Errorf("load system trust store: %w", err)
	}
	if roots == nil {
		return false, nil
	}
	_, err = cert.Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	if err == nil {
		return true, nil
	}
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &unknown) {
		return false, nil
	}
	return false, nil
}

type systemBackend struct {
	name       string
	anchorDir  string
	extension  string
	refreshCmd []string
}

func DetectSystemBackend() (Backend, error) {
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("automatic host trust installation is not supported on %s; use 'baha trust export' and install the public CA with the platform trust tooling", runtime.GOOS)
	}
	if path, err := exec.LookPath("update-ca-certificates"); err == nil && path != "" {
		return &systemBackend{
			name: "linux-update-ca-certificates", anchorDir: "/usr/local/share/ca-certificates",
			extension: ".crt", refreshCmd: []string{"update-ca-certificates"},
		}, nil
	}
	if path, err := exec.LookPath("update-ca-trust"); err == nil && path != "" {
		return &systemBackend{
			name: "linux-update-ca-trust", anchorDir: "/etc/pki/ca-trust/source/anchors",
			extension: ".pem", refreshCmd: []string{"update-ca-trust", "extract"},
		}, nil
	}
	return nil, errors.New("no supported Linux system trust updater found; use 'baha trust export' and install the public CA manually")
}

var resolveBackend = backendByName

func backendByName(name string) (Backend, error) {
	switch name {
	case "linux-update-ca-certificates":
		return &systemBackend{name: name, anchorDir: "/usr/local/share/ca-certificates", extension: ".crt", refreshCmd: []string{"update-ca-certificates"}}, nil
	case "linux-update-ca-trust":
		return &systemBackend{name: name, anchorDir: "/etc/pki/ca-trust/source/anchors", extension: ".pem", refreshCmd: []string{"update-ca-trust", "extract"}}, nil
	default:
		return nil, fmt.Errorf("unsupported recorded host trust backend %q", name)
	}
}

func (b *systemBackend) Name() string { return b.name }

func (b *systemBackend) AnchorPath(fingerprint string) (string, error) {
	if len(fingerprint) != 64 {
		return "", errors.New("invalid CA fingerprint for host trust anchor")
	}
	for _, r := range fingerprint {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", errors.New("invalid CA fingerprint for host trust anchor")
		}
	}
	return filepath.Join(b.anchorDir, "baseharbor-"+fingerprint[:16]+b.extension), nil
}

func (b *systemBackend) Install(ctx context.Context, source, target string) error {
	if err := b.validateOwnedTarget(target); err != nil {
		return err
	}
	if err := runMaybePrivileged(ctx, "install", "-m", "0644", source, target); err != nil {
		return fmt.Errorf("install BaseHarbor host trust anchor: %w", err)
	}
	if err := runMaybePrivileged(ctx, b.refreshCmd[0], b.refreshCmd[1:]...); err != nil {
		_ = runMaybePrivileged(ctx, "rm", "-f", target)
		return fmt.Errorf("refresh system trust store: %w", err)
	}
	return nil
}

func (b *systemBackend) Remove(ctx context.Context, target string) error {
	if err := b.validateOwnedTarget(target); err != nil {
		return err
	}
	if err := runMaybePrivileged(ctx, "rm", "-f", target); err != nil {
		return err
	}
	if err := runMaybePrivileged(ctx, b.refreshCmd[0], b.refreshCmd[1:]...); err != nil {
		return fmt.Errorf("refresh system trust store after removal: %w", err)
	}
	return nil
}

func (b *systemBackend) validateOwnedTarget(target string) error {
	clean := filepath.Clean(target)
	dir := filepath.Clean(b.anchorDir)
	if filepath.Dir(clean) != dir {
		return errors.New("recorded host trust path is outside the supported system anchor directory")
	}
	base := filepath.Base(clean)
	if !strings.HasPrefix(base, "baseharbor-") || !strings.HasSuffix(base, b.extension) {
		return errors.New("recorded host trust path is not a BaseHarbor-owned anchor")
	}
	return nil
}

func runMaybePrivileged(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if out, err := cmd.CombinedOutput(); err == nil {
		return nil
	} else {
		if _, sudoErr := exec.LookPath("sudo"); sudoErr != nil {
			return fmt.Errorf("%s failed and sudo is unavailable: %w: %s", name, err, strings.TrimSpace(string(out)))
		}
		sudoArgs := append([]string{"--", name}, args...)
		sudo := exec.CommandContext(ctx, "sudo", sudoArgs...)
		sudo.Stdin = os.Stdin
		sudo.Stdout = os.Stdout
		sudo.Stderr = os.Stderr
		if sudoErr := sudo.Run(); sudoErr != nil {
			return fmt.Errorf("sudo %s failed: %w", name, sudoErr)
		}
		return nil
	}
}
