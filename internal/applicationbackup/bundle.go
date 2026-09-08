package applicationbackup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	SchemaVersion        = 1
	maxArchiveEntries    = 64
	maxArchivePlainBytes = int64(20 << 30)
)

type Metadata struct {
	SchemaVersion int      `json:"schema_version"`
	Application   string   `json:"application"`
	Environment   string   `json:"environment"`
	CreatedAt     string   `json:"created_at"`
	Postgres      []string `json:"postgres_instances"`
}

type EntryIntegrity struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Index struct {
	SchemaVersion int                       `json:"schema_version"`
	Entries       map[string]EntryIntegrity `json:"entries"`
}

type countingHashWriter struct {
	w    io.Writer
	hash hashWriter
	n    int64
}

type hashWriter interface {
	Write([]byte) (int, error)
	Sum([]byte) []byte
}

func (w *countingHashWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		_, _ = w.hash.Write(p[:n])
		w.n += int64(n)
	}
	return n, err
}

func Create(ctx context.Context, outputPath string, key []byte, manifest application.Manifest, manifestYAML []byte, secrets openbao.ApplicationSecretBackup, compose bhruntime.Compose, runtimeFiles application.RuntimeFiles) error {
	if len(key) != 32 {
		return errors.New("backup key must be 32 bytes")
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if secrets.Values == nil && manifest.Services.Secrets {
		return errors.New("managed secrets are enabled but no secret backup was supplied")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil && filepath.Dir(outputPath) != "." {
		return fmt.Errorf("create backup output directory: %w", err)
	}
	f, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create backup file: %w", err)
	}
	keep := false
	defer func() {
		_ = f.Close()
		if !keep {
			_ = os.Remove(outputPath)
		}
	}()

	encrypted, err := NewEncryptWriter(f, key)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(encrypted)
	integrity := map[string]EntryIntegrity{}

	postgresTargets := application.PostgresBackupTargets(manifest)
	instances := make([]string, 0, len(postgresTargets))
	for _, target := range postgresTargets {
		instances = append(instances, target.Instance)
	}
	metadata := Metadata{SchemaVersion: SchemaVersion, Application: manifest.Name, Environment: manifest.Environment, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Postgres: instances}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if err := writeBytesEntry(zw, "metadata.json", metadataJSON, integrity); err != nil {
		return err
	}
	if err := writeBytesEntry(zw, "manifest.yaml", manifestYAML, integrity); err != nil {
		return err
	}
	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		return errors.New("encode application secret backup")
	}
	if err := writeBytesEntry(zw, "secrets.json", secretsJSON, integrity); err != nil {
		return err
	}

	for _, target := range postgresTargets {
		name := "postgres/" + target.Instance + ".dump"
		entry, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return err
		}
		h := sha256.New()
		cw := &countingHashWriter{w: entry, hash: h}
		if err := compose.ExecProjectStream(ctx, application.RuntimeProjectName(manifest), runtimeFiles.Compose, runtimeFiles.Env, nil, cw, target.Service, "pg_dump", "-U", "baseharbor", "-d", target.Database, "--format=custom", "--no-owner", "--no-privileges"); err != nil {
			return fmt.Errorf("backup PostgreSQL instance %s: %w", target.Instance, err)
		}
		if cw.n == 0 {
			return fmt.Errorf("backup PostgreSQL instance %s produced an empty dump", target.Instance)
		}
		integrity[name] = EntryIntegrity{SHA256: hex.EncodeToString(h.Sum(nil)), Size: cw.n}
	}

	index := Index{SchemaVersion: SchemaVersion, Entries: integrity}
	indexJSON, err := json.Marshal(index)
	if err != nil {
		return err
	}
	entry, err := zw.CreateHeader(&zip.FileHeader{Name: "index.json", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := entry.Write(indexJSON); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finalize backup archive: %w", err)
	}
	if err := encrypted.Close(); err != nil {
		return fmt.Errorf("finalize backup encryption: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync backup file: %w", err)
	}
	if err := f.Close(); err != nil {
		return err
	}
	keep = true
	return nil
}

func writeBytesEntry(zw *zip.Writer, name string, data []byte, integrity map[string]EntryIntegrity) error {
	entry, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		return err
	}
	h := sha256.Sum256(data)
	if _, err := entry.Write(data); err != nil {
		return err
	}
	integrity[name] = EntryIntegrity{SHA256: hex.EncodeToString(h[:]), Size: int64(len(data))}
	return nil
}

type PreparedRestore struct {
	Metadata Metadata
	Secrets  openbao.ApplicationSecretBackup
	Manifest []byte
	zipPath  string
	archive  *zip.ReadCloser
	files    map[string]*zip.File
}

func PrepareRestore(backupPath, keyPath string) (*PreparedRestore, error) {
	key, _, err := LoadBackupKey(keyPath, false)
	if err != nil {
		return nil, err
	}
	input, err := os.Open(backupPath)
	if err != nil {
		return nil, fmt.Errorf("open backup: %w", err)
	}
	defer input.Close()
	decrypt, err := NewDecryptReader(input, key)
	if err != nil {
		return nil, err
	}
	temp, err := os.CreateTemp("", "baseharbor-restore-*.zip")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	keep := false
	defer func() {
		_ = temp.Close()
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	if err := os.Chmod(tempPath, 0o600); err != nil {
		return nil, err
	}
	limited := &io.LimitedReader{R: decrypt, N: maxArchivePlainBytes + 1}
	n, err := io.Copy(temp, limited)
	if err != nil {
		return nil, fmt.Errorf("decrypt backup: %w", err)
	}
	if n > maxArchivePlainBytes {
		return nil, errors.New("backup exceeds maximum decrypted size")
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(tempPath)
	if err != nil {
		return nil, errors.New("decrypted backup is not a valid archive")
	}
	prepared := &PreparedRestore{zipPath: tempPath, archive: zr, files: map[string]*zip.File{}}
	if err := prepared.validate(); err != nil {
		prepared.Close()
		return nil, err
	}
	keep = true
	return prepared, nil
}

func (p *PreparedRestore) Close() error {
	if p == nil {
		return nil
	}
	if p.archive != nil {
		_ = p.archive.Close()
		p.archive = nil
	}
	if p.zipPath != "" {
		err := os.Remove(p.zipPath)
		p.zipPath = ""
		return err
	}
	return nil
}

func (p *PreparedRestore) validate() error {
	if len(p.archive.File) == 0 || len(p.archive.File) > maxArchiveEntries {
		return errors.New("backup archive contains an invalid number of entries")
	}
	var total uint64
	for _, f := range p.archive.File {
		if f.FileInfo().Mode()&os.ModeSymlink != 0 || !f.FileInfo().Mode().IsRegular() {
			return errors.New("backup archive contains a non-regular entry")
		}
		if !validArchiveName(f.Name) {
			return fmt.Errorf("backup archive contains an unexpected entry %q", f.Name)
		}
		if _, exists := p.files[f.Name]; exists {
			return fmt.Errorf("backup archive contains duplicate entry %q", f.Name)
		}
		p.files[f.Name] = f
		total += f.UncompressedSize64
		if total > uint64(maxArchivePlainBytes) {
			return errors.New("backup archive expands beyond the allowed size")
		}
	}
	metadataBytes, err := p.readSmall("metadata.json", 1<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(metadataBytes, &p.Metadata); err != nil || p.Metadata.SchemaVersion != SchemaVersion || p.Metadata.Application == "" || p.Metadata.Environment == "" {
		return errors.New("backup metadata is invalid or unsupported")
	}
	indexBytes, err := p.readSmall("index.json", 4<<20)
	if err != nil {
		return err
	}
	var index Index
	if err := json.Unmarshal(indexBytes, &index); err != nil || index.SchemaVersion != SchemaVersion || index.Entries == nil {
		return errors.New("backup integrity index is invalid or unsupported")
	}
	if len(index.Entries) != len(p.files)-1 {
		return errors.New("backup integrity index does not cover every payload entry")
	}
	for name, expected := range index.Entries {
		f, ok := p.files[name]
		if !ok || name == "index.json" {
			return errors.New("backup integrity index references an invalid entry")
		}
		if int64(f.UncompressedSize64) != expected.Size {
			return fmt.Errorf("backup checksum size mismatch for %s", name)
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, r)
		_ = r.Close()
		if copyErr != nil || hex.EncodeToString(h.Sum(nil)) != expected.SHA256 {
			return fmt.Errorf("backup checksum mismatch for %s", name)
		}
	}
	manifest, err := p.readSmall("manifest.yaml", 4<<20)
	if err != nil {
		return err
	}
	p.Manifest = manifest
	secretsBytes, err := p.readSmall("secrets.json", 64<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(secretsBytes, &p.Secrets); err != nil || p.Secrets.Values == nil {
		return errors.New("backup secret payload is invalid")
	}
	expectedPG := append([]string(nil), p.Metadata.Postgres...)
	sort.Strings(expectedPG)
	actualPG := []string{}
	for name := range p.files {
		if strings.HasPrefix(name, "postgres/") && strings.HasSuffix(name, ".dump") {
			actualPG = append(actualPG, strings.TrimSuffix(strings.TrimPrefix(name, "postgres/"), ".dump"))
		}
	}
	sort.Strings(actualPG)
	if strings.Join(expectedPG, "\x00") != strings.Join(actualPG, "\x00") {
		return errors.New("backup PostgreSQL entry set does not match metadata")
	}
	return nil
}

func validArchiveName(name string) bool {
	if name == "metadata.json" || name == "manifest.yaml" || name == "secrets.json" || name == "index.json" {
		return true
	}
	if !strings.HasPrefix(name, "postgres/") || !strings.HasSuffix(name, ".dump") {
		return false
	}
	instance := strings.TrimSuffix(strings.TrimPrefix(name, "postgres/"), ".dump")
	if instance == "" || strings.Contains(instance, "/") || strings.Contains(instance, "\\") || strings.Contains(instance, "..") {
		return false
	}
	for _, r := range instance {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

func (p *PreparedRestore) readSmall(name string, limit int64) ([]byte, error) {
	f, ok := p.files[name]
	if !ok {
		return nil, fmt.Errorf("backup is missing %s", name)
	}
	if int64(f.UncompressedSize64) > limit {
		return nil, fmt.Errorf("backup entry %s exceeds its size limit", name)
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, limit+1))
}

func (p *PreparedRestore) PostgresReader(instance string) (io.ReadCloser, error) {
	if p == nil || p.archive == nil {
		return nil, errors.New("prepared restore is closed")
	}
	f, ok := p.files["postgres/"+instance+".dump"]
	if !ok {
		return nil, fmt.Errorf("backup is missing PostgreSQL instance %s", instance)
	}
	return f.Open()
}
