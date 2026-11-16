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
	LastUpdateMetadataVersion = 1
	lastUpdateMetadataName    = "last-update.json"
)

var ErrNoUpdateMetadata = errors.New("no application update metadata recorded")

type UpdateMetadata struct {
	Version         int       `json:"version"`
	Application     string    `json:"application"`
	Environment     string    `json:"environment"`
	UpdatedAt       time.Time `json:"updated_at"`
	Branch          string    `json:"branch"`
	Upstream        string    `json:"upstream"`
	FromRevision    string    `json:"from_revision"`
	ToRevision      string    `json:"to_revision"`
	Result          string    `json:"result"`
	BackupPath      string    `json:"backup_path,omitempty"`
	BackupCreatedAt time.Time `json:"backup_created_at,omitempty"`
}

func (m UpdateMetadata) Validate() error {
	if m.Version != LastUpdateMetadataVersion {
		return fmt.Errorf("unsupported update metadata version %d", m.Version)
	}
	if err := validateSlug("application name", m.Application); err != nil {
		return err
	}
	if strings.TrimSpace(m.Environment) == "" {
		return errors.New("update metadata environment is required")
	}
	if m.UpdatedAt.IsZero() {
		return errors.New("update metadata timestamp is required")
	}
	if strings.TrimSpace(m.Branch) == "" || strings.TrimSpace(m.Upstream) == "" {
		return errors.New("update metadata Git branch and upstream are required")
	}
	if strings.TrimSpace(m.FromRevision) == "" || strings.TrimSpace(m.ToRevision) == "" {
		return errors.New("update metadata revisions are required")
	}
	switch m.Result {
	case "ready", "runtime-verification-failed":
	default:
		return fmt.Errorf("unsupported update result %q", m.Result)
	}
	if m.BackupPath == "" && !m.BackupCreatedAt.IsZero() {
		return errors.New("update metadata backup timestamp requires a backup path")
	}
	if m.BackupPath != "" && m.BackupCreatedAt.IsZero() {
		return errors.New("update metadata backup path requires a backup timestamp")
	}
	return nil
}

func (s Store) RecordLastUpdate(metadata UpdateMetadata) error {
	if err := metadata.Validate(); err != nil {
		return err
	}
	appDir := filepath.Join(s.Root, metadata.Application)
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return fmt.Errorf("create application state directory for update metadata: %w", err)
	}
	path := filepath.Join(appDir, lastUpdateMetadataName)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("encode application update metadata: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write application update metadata: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("protect application update metadata: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit application update metadata: %w", err)
	}
	return nil
}

func (s Store) LastUpdate(applicationName string) (UpdateMetadata, error) {
	if err := validateSlug("application name", applicationName); err != nil {
		return UpdateMetadata{}, err
	}
	path := filepath.Join(s.Root, applicationName, lastUpdateMetadataName)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return UpdateMetadata{}, ErrNoUpdateMetadata
	}
	if err != nil {
		return UpdateMetadata{}, fmt.Errorf("read application update metadata: %w", err)
	}
	var metadata UpdateMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return UpdateMetadata{}, fmt.Errorf("decode application update metadata: %w", err)
	}
	if err := metadata.Validate(); err != nil {
		return UpdateMetadata{}, fmt.Errorf("validate application update metadata: %w", err)
	}
	if metadata.Application != applicationName {
		return UpdateMetadata{}, errors.New("application update metadata identity does not match state path")
	}
	return metadata, nil
}
