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
	LastRecoveryMetadataVersion = 1
	lastRecoveryMetadataName    = "last-recovery.json"
)

var ErrNoRecoveryMetadata = errors.New("no application recovery metadata recorded")

type RecoveryMetadata struct {
	Version         int       `json:"version"`
	Application     string    `json:"application"`
	Environment     string    `json:"environment"`
	RestoredAt      time.Time `json:"restored_at"`
	BackupCreatedAt time.Time `json:"backup_created_at"`
	ArchivePath     string    `json:"archive_path"`
}

func (m RecoveryMetadata) Validate() error {
	if m.Version != LastRecoveryMetadataVersion {
		return fmt.Errorf("unsupported recovery metadata version %d", m.Version)
	}
	if err := validateSlug("application name", m.Application); err != nil {
		return err
	}
	if strings.TrimSpace(m.Environment) == "" {
		return errors.New("recovery metadata environment is required")
	}
	if m.RestoredAt.IsZero() {
		return errors.New("recovery metadata restore timestamp is required")
	}
	if m.BackupCreatedAt.IsZero() {
		return errors.New("recovery metadata backup creation timestamp is required")
	}
	if strings.TrimSpace(m.ArchivePath) == "" {
		return errors.New("recovery metadata archive path is required")
	}
	return nil
}

func (s Store) RecordLastRecovery(metadata RecoveryMetadata) error {
	if err := metadata.Validate(); err != nil {
		return err
	}
	appDir := filepath.Join(s.Root, metadata.Application)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return fmt.Errorf("create application state directory for recovery metadata: %w", err)
	}
	path := filepath.Join(appDir, lastRecoveryMetadataName)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode application recovery metadata: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write application recovery metadata: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("protect application recovery metadata: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit application recovery metadata: %w", err)
	}
	return nil
}

func (s Store) LastRecovery(applicationName string) (RecoveryMetadata, error) {
	if err := validateSlug("application name", applicationName); err != nil {
		return RecoveryMetadata{}, err
	}
	path := filepath.Join(s.Root, applicationName, lastRecoveryMetadataName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return RecoveryMetadata{}, ErrNoRecoveryMetadata
	}
	if err != nil {
		return RecoveryMetadata{}, fmt.Errorf("read application recovery metadata: %w", err)
	}
	var metadata RecoveryMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return RecoveryMetadata{}, fmt.Errorf("decode application recovery metadata: %w", err)
	}
	if err := metadata.Validate(); err != nil {
		return RecoveryMetadata{}, fmt.Errorf("validate application recovery metadata: %w", err)
	}
	if metadata.Application != applicationName {
		return RecoveryMetadata{}, errors.New("application recovery metadata identity does not match state path")
	}
	return metadata, nil
}
