package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type applicationRestoreData struct {
	manifest        application.Manifest
	postgresBackups []application.PostgresBackup
	secretBackup    openbao.ApplicationSecretBackup
}

func captureApplicationBackup(ctx context.Context, compose bhruntime.Compose, platformFiles bhruntime.Files, m application.Manifest, files application.RuntimeFiles, password []byte, outputPath string) error {
	entries := make([]applicationbackup.PayloadEntry, 0, 2+len(application.SQLInstanceNames(m)))
	metadata, err := applicationbackup.ApplicationManifestPayloadEntry(m)
	if err != nil {
		return err
	}
	entries = append(entries, metadata)

	dumps, err := application.DumpPostgresInstances(ctx, compose, m, files)
	if err != nil {
		return err
	}
	postgresEntries, err := applicationbackup.PostgresPayloadEntries(dumps)
	if err != nil {
		return err
	}
	entries = append(entries, postgresEntries...)

	if m.Services.Secrets {
		identity := openbao.ApplicationIdentity{Name: m.Name, Environment: m.Environment}
		secretBackup, err := openbao.ExportApplicationSecrets(ctx, compose, platformFiles, identity, openbao.ApplicationCredentialsPath(files.Dir))
		if err != nil {
			return err
		}
		secretEntry, err := applicationbackup.OpenBaoPayloadEntry(secretBackup)
		if err != nil {
			return err
		}
		entries = append(entries, secretEntry)
	}

	archive, err := applicationbackup.Build(m.Name, m.Environment, time.Now().UTC(), entries, password)
	if err != nil {
		return err
	}
	defer zeroBytes(archive)
	return writeBackupArchive(outputPath, archive)
}

func loadApplicationRestoreData(backupPath string, password []byte, name, environment string) (applicationRestoreData, error) {
	archive, err := os.ReadFile(backupPath)
	if err != nil {
		return applicationRestoreData{}, fmt.Errorf("read application backup: %w", err)
	}
	defer zeroBytes(archive)

	payload, err := applicationbackup.Open(archive, password)
	if err != nil {
		return applicationRestoreData{}, fmt.Errorf("validate application backup before mutation: %w", err)
	}
	m, err := applicationbackup.ApplicationManifestFromPayload(payload)
	if err != nil {
		return applicationRestoreData{}, fmt.Errorf("validate application metadata before mutation: %w", err)
	}
	if name != "" && name != m.Name {
		return applicationRestoreData{}, errors.New("restore target NAME does not match backup application identity")
	}
	if environment != "" && environment != m.Environment {
		return applicationRestoreData{}, fmt.Errorf("restore target environment %q does not match backup environment %q", environment, m.Environment)
	}
	if application.HasObjectStorage(m) {
		return applicationRestoreData{}, errors.New("application restore does not yet restore object-storage contents; refusing an incomplete recovery")
	}

	postgresBackups, err := applicationbackup.PostgresBackupsFromPayload(m, payload)
	if err != nil {
		return applicationRestoreData{}, fmt.Errorf("validate PostgreSQL backup before mutation: %w", err)
	}

	var secretBackup openbao.ApplicationSecretBackup
	if m.Services.Secrets {
		secretBackup, err = applicationbackup.OpenBaoBackupFromPayload(m.Name, m.Environment, payload)
		if err != nil {
			return applicationRestoreData{}, fmt.Errorf("validate OpenBao backup before mutation: %w", err)
		}
	}
	return applicationRestoreData{manifest: m, postgresBackups: postgresBackups, secretBackup: secretBackup}, nil
}
