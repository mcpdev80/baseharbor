package hosttrust

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const nssBackendName = "linux-browser-nss"

type BrowserStatus struct {
	Database string `json:"database,omitempty"`
	State    string `json:"state"`
	Detail   string `json:"detail,omitempty"`
}

// InstallBrowsers adds only the requested public CA to existing Linux browser
// databases. It never creates profiles, changes browser policies or overwrites
// a conflicting certificate. Owned entries share host trust removal tracking.
func InstallBrowsers(ctx context.Context, stateDir string, publicCA []byte) ([]BrowserStatus, error) {
	if runtime.GOOS != "linux" {
		return []BrowserStatus{{State: "manual", Detail: "Browser trust is platform-specific; import the exported public CA in your browser."}}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	databases := browserNSSDatabases(home)
	if len(databases) == 0 {
		return []BrowserStatus{{State: "not-found", Detail: "No existing supported browser certificate databases found."}}, nil
	}
	if _, err := exec.LookPath("certutil"); err != nil {
		return []BrowserStatus{{State: "manual", Detail: "Install NSS certutil (Debian/Ubuntu: libnss3-tools), then retry trust install."}}, nil
	}
	var result []BrowserStatus
	for _, db := range databases {
		status := BrowserStatus{Database: db, State: "trusted"}
		if err := installBrowserCA(ctx, stateDir, publicCA, db); err != nil {
			status.State = "manual"
			status.Detail = err.Error()
		}
		result = append(result, status)
	}
	return result, nil
}

func browserNSSDatabases(home string) []string {
	patterns := []string{
		".pki/nssdb/cert9.db", ".local/share/pki/nssdb/cert9.db",
		".mozilla/firefox/*/cert9.db", "snap/firefox/common/.mozilla/firefox/*/cert9.db",
		".var/app/org.mozilla.firefox/.mozilla/firefox/*/cert9.db",
	}
	var result []string
	seen := map[string]bool{}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(home, pattern))
		for _, path := range matches {
			db := filepath.Dir(path)
			if validNSSDatabase(db) == nil && !seen[db] {
				result = append(result, db)
				seen[db] = true
			}
		}
	}
	return result
}

func validNSSDatabase(database string) error {
	if !filepath.IsAbs(database) {
		return errors.New("browser database path must be absolute")
	}
	// Do not follow profile/database symlinks into another user's certificate store.
	for path := filepath.Join(database, "cert9.db"); ; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("browser certificate database has a symlinked path")
		}
		if path == filepath.Join(database, "cert9.db") && !info.Mode().IsRegular() {
			return errors.New("browser certificate database is not a regular file")
		}
		if path == filepath.Dir(path) {
			break
		}
	}
	return nil
}

var runNSSCommand = func(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "certutil", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("NSS certificate operation failed; close the browser and retry (certutil: %w)", err)
	}
	return output, nil
}

type nssBackend struct{ database, fingerprint string }

func (b *nssBackend) Name() string     { return nssBackendName }
func (b *nssBackend) nickname() string { return "BaseHarbor " + b.fingerprint }
func (b *nssBackend) AnchorPath(fingerprint string) (string, error) {
	if len(fingerprint) != 64 || strings.Trim(fingerprint, "0123456789abcdef") != "" {
		return "", errors.New("invalid browser CA fingerprint")
	}
	b.fingerprint = fingerprint
	if err := validNSSDatabase(b.database); err != nil {
		return "", err
	}
	return b.database, nil
}
func (b *nssBackend) export(ctx context.Context) ([]byte, error) {
	if err := validNSSDatabase(b.database); err != nil {
		return nil, err
	}
	listing, err := runNSSCommand(ctx, "-L", "-d", "sql:"+b.database)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(string(listing), b.nickname()) {
		return nil, os.ErrNotExist
	}
	return runNSSCommand(ctx, "-L", "-d", "sql:"+b.database, "-n", b.nickname(), "-a")
}
func (b *nssBackend) trusted(ctx context.Context) error {
	_, err := runNSSCommand(ctx, "-V", "-d", "sql:"+b.database, "-n", b.nickname(), "-u", "L")
	return err
}
func (b *nssBackend) Install(ctx context.Context, source, target string) error {
	if target != b.database {
		return errors.New("browser database mismatch")
	}
	if _, err := b.export(ctx); !errors.Is(err, os.ErrNotExist) {
		return errors.New("browser certificate already exists; refusing overwrite")
	}
	_, err := runNSSCommand(ctx, "-A", "-d", "sql:"+b.database, "-n", b.nickname(), "-t", "C,,", "-i", source)
	return err
}
func (b *nssBackend) Remove(ctx context.Context, target string) error {
	if target != b.database {
		return errors.New("browser database mismatch")
	}
	data, err := b.export(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, fingerprint, err := ParseCA(data)
	if err != nil || fingerprint != b.fingerprint {
		return errors.New("browser certificate changed; refusing removal")
	}
	_, err = runNSSCommand(ctx, "-D", "-d", "sql:"+b.database, "-n", b.nickname())
	return err
}

func installBrowserCA(ctx context.Context, stateDir string, publicCA []byte, database string) error {
	_, fingerprint, err := ParseCA(publicCA)
	if err != nil {
		return err
	}
	backend := &nssBackend{database: database}
	if _, err := backend.AnchorPath(fingerprint); err != nil {
		return err
	}
	state, err := loadState(stateDir)
	if err != nil {
		return err
	}
	existing, err := backend.export(ctx)
	if err == nil {
		_, actual, parseErr := ParseCA(existing)
		if parseErr != nil || actual != fingerprint {
			return errors.New("conflicting browser certificate; left unchanged")
		}
		return backend.trusted(ctx) // Do not claim an operator-installed certificate.
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(stateDir, "browser-public-ca-*.pem")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(publicCA); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := backend.Install(ctx, file.Name(), database); err != nil {
		return err
	}
	if err := backend.trusted(ctx); err != nil {
		_ = backend.Remove(ctx, database)
		return err
	}
	// Reinstallation of a missing owned certificate updates its existing record.
	found := false
	for _, record := range state.Anchors {
		if record.Backend == nssBackendName && record.Path == database && record.Fingerprint == fingerprint {
			found = true
		}
	}
	if !found {
		state.Anchors = append(state.Anchors, AnchorRecord{Fingerprint: fingerprint, Backend: nssBackendName, Path: database, InstalledAt: time.Now().UTC()})
	}
	if err := saveState(stateDir, state); err != nil {
		_ = backend.Remove(ctx, database)
		return err
	}
	return nil
}

func readRecordedNSSCA(record AnchorRecord) ([]byte, error) {
	backend := &nssBackend{database: record.Path}
	if _, err := backend.AnchorPath(record.Fingerprint); err != nil {
		return nil, err
	}
	data, err := backend.export(context.Background())
	if err != nil {
		return nil, err
	}
	_, fingerprint, err := ParseCA(data)
	if err != nil || fingerprint != record.Fingerprint {
		return nil, errors.New("recorded browser CA fingerprint changed; certificate preserved")
	}
	return data, nil
}

func verifyNSSRecord(record AnchorRecord) error {
	_, err := readRecordedNSSCA(record)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
