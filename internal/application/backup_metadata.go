package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LastBackupMetadataVersion = 1
	lastBackupMetadataName    = "last-backup.json"
)

var ErrNoBackupMetadata = errors.New("no application backup metadata recorded")

type BackupMetadata struct {
	Version          int       `json:"version"`
	Application      string    `json:"application"`
	Environment      string    `json:"environment"`
	CreatedAt        time.Time `json:"created_at"`
	ArchivePath      string    `json:"archive_path"`
	PostgresResources []string `json:"postgres_resources,omitempty"`
	IncludesSecrets  bool      `json:"includes_secrets"`
}

func (m BackupMetadata) Validate() error {
	if m.Version != LastBackupMetadataVersion {
		return fmt.Errorf("unsupported backup metadata version %d", m.Version)
	}
	if err := validateSlug("application name", m.Application); err != nil {
		return err
	}
	if strings.TrimSpace(m.Environment) == "" {
		return errors.New("backup metadata environment is required")
	}
	if m.CreatedAt.IsZero() {
		return errors.New("backup metadata creation timestamp is required")
	}
	if strings.TrimSpace(m.ArchivePath) == "" {
		return errors.New("backup metadata archive path is required")
	}
	for _, name := range m.PostgresResources {
		if err := validateSlug("PostgreSQL resource name", name); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) RecordLastBackup(metadata BackupMetadata) error {
	if err := metadata.Validate(); err != nil {
		return err
	}
	appDir := filepath.Join(s.Root, metadata.Application)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return fmt.Errorf("create application state directory for backup metadata: %w", err)
	}
	path := filepath.Join(appDir, lastBackupMetadataName)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode application backup metadata: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write application backup metadata: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("protect application backup metadata: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit application backup metadata: %w", err)
	}
	return nil
}

func (s Store) LastBackup(applicationName string) (BackupMetadata, error) {
	if err := validateSlug("application name", applicationName); err != nil {
		return BackupMetadata{}, err
	}
	path := filepath.Join(s.Root, applicationName, lastBackupMetadataName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return BackupMetadata{}, ErrNoBackupMetadata
	}
	if err != nil {
		return BackupMetadata{}, fmt.Errorf("read application backup metadata: %w", err)
	}
	var metadata BackupMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return BackupMetadata{}, fmt.Errorf("decode application backup metadata: %w", err)
	}
	if err := metadata.Validate(); err != nil {
		return BackupMetadata{}, fmt.Errorf("validate application backup metadata: %w", err)
	}
	if metadata.Application != applicationName {
		return BackupMetadata{}, errors.New("application backup metadata identity does not match state path")
	}
	return metadata, nil
}
