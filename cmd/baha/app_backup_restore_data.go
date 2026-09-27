package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	logsprovider "github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type applicationRestoreData struct {
	manifest         application.Manifest
	recoveryManifest applicationbackup.RecoveryManifest
	postgresBackups  []application.PostgresBackup
	secretBackup     openbao.ApplicationSecretBackup
	objectStorage    []objectstorage.BucketBackup
	workloadStorage  map[string][]byte
	logsHistory      logsprovider.HistoryBackup
}

func captureApplicationBackup(ctx context.Context, compose bhruntime.Compose, platformFiles bhruntime.Files, resolved resolvedApplication, files application.RuntimeFiles, selectionArgs recoverySelectionArgs, password []byte, outputPath string) error {
	m := resolved.Manifest
	selection, workloadVolumes, err := discoverApplicationRecoverySelection(ctx, compose, resolved, files)
	if err != nil {
		return fmt.Errorf("discover recovery contributors: %w", err)
	}
	selection, err = selection.Apply(selectionArgs.Include, selectionArgs.Exclude)
	if err != nil {
		return err
	}
	if err := selection.ValidateForCapture(); err != nil {
		return err
	}
	entries := make([]applicationbackup.PayloadEntry, 0, 3+len(application.SQLInstanceNames(m))+len(application.ObjectStorageBucketNames(m))+len(workloadVolumes))
	metadata, err := applicationbackup.ApplicationManifestPayloadEntry(m)
	if err != nil {
		return err
	}
	entries = append(entries, metadata)
	recoveryMetadata, err := applicationbackup.RecoveryManifestPayloadEntry(selection)
	if err != nil {
		return fmt.Errorf("encode recovery manifest: %w", err)
	}
	entries = append(entries, recoveryMetadata)

	if selection.HasSelected(applicationbackup.StateSQL) {
		dumps, err := application.DumpPostgresInstancesAt(ctx, compose, m, files, resolved.TargetStateRoot, resolved.Target.Name)
		if err != nil {
			return err
		}
		postgresEntries, err := applicationbackup.PostgresPayloadEntries(dumps)
		if err != nil {
			return err
		}
		entries = append(entries, postgresEntries...)
	}
	if m.Services.Secrets && selection.HasSelected(applicationbackup.StateSecrets) {
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
	if selection.HasSelected(applicationbackup.StateObjectStorage) {
		driver := objectstorage.NewDriverAt(compose, m, files, nil, resolved.TargetStateRoot, resolved.Target.Name)
		for _, bucket := range application.ObjectStorageBucketNames(m) {
			backup, err := driver.ExportBucket(ctx, bucket)
			if err != nil {
				return err
			}
			entry, err := applicationbackup.ObjectStoragePayloadEntry(backup)
			if err != nil {
				return err
			}
			entries = append(entries, entry)
		}
	}
	if selection.HasSelected(applicationbackup.StateLogs) {
		history, err := logsprovider.ExportApplicationHistoryAt(ctx, m, resolved.TargetStateRoot, resolved.Target.Name)
		if err != nil {
			return fmt.Errorf("capture application log history: %w", err)
		}
		entry, err := applicationbackup.LogsHistoryPayloadEntry(history)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
	}
	if selection.HasSelected(applicationbackup.StateWorkloadStorage) {
		for _, volume := range workloadVolumes {
			archive, err := compose.ExportOwnedVolume(ctx, volume.Project, volume.Volume)
			if err != nil {
				return fmt.Errorf("capture workload volume %s: %w", volume.Logical, err)
			}
			entry, err := applicationbackup.WorkloadStoragePayloadEntry(volume.Logical, archive)
			zeroBytes(archive)
			if err != nil {
				return err
			}
			entries = append(entries, entry)
		}
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
	recoveryManifest, found, err := applicationbackup.RecoveryManifestFromPayload(payload)
	if err != nil {
		return applicationRestoreData{}, fmt.Errorf("validate recovery manifest before mutation: %w", err)
	}
	if !found {
		legacySelection, discoveryErr := applicationbackup.DiscoverManifestRecovery(m)
		if discoveryErr != nil {
			return applicationRestoreData{}, fmt.Errorf("derive legacy recovery manifest: %w", discoveryErr)
		}
		recoveryManifest = applicationbackup.RecoveryManifest{
			Version:      applicationbackup.RecoveryManifestVersion,
			Contributors: legacySelection.Contributors,
		}
	}

	var postgresBackups []application.PostgresBackup
	if recoveryManifestHasSelected(recoveryManifest, applicationbackup.StateSQL) {
		postgresBackups, err = applicationbackup.PostgresBackupsFromPayload(m, payload)
		if err != nil {
			return applicationRestoreData{}, fmt.Errorf("validate PostgreSQL backup before mutation: %w", err)
		}
	}
	var secretBackup openbao.ApplicationSecretBackup
	if m.Services.Secrets && recoveryManifestHasSelected(recoveryManifest, applicationbackup.StateSecrets) {
		secretBackup, err = applicationbackup.OpenBaoBackupFromPayload(m.Name, m.Environment, payload)
		if err != nil {
			return applicationRestoreData{}, fmt.Errorf("validate OpenBao backup before mutation: %w", err)
		}
	}
	var objectBackups []objectstorage.BucketBackup
	if recoveryManifestHasSelected(recoveryManifest, applicationbackup.StateObjectStorage) {
		objectBackups, err = applicationbackup.ObjectStorageBackupsFromPayload(payload)
		if err != nil {
			return applicationRestoreData{}, fmt.Errorf("validate object-storage backup before mutation: %w", err)
		}
		if err := validateRecoveredLogicalResources(recoveryManifest, applicationbackup.StateObjectStorage, bucketBackupNames(objectBackups)); err != nil {
			return applicationRestoreData{}, err
		}
	}
	var workloadStorage map[string][]byte
	if recoveryManifestHasSelected(recoveryManifest, applicationbackup.StateWorkloadStorage) {
		workloadStorage, err = applicationbackup.WorkloadStorageFromPayload(payload)
		if err != nil {
			return applicationRestoreData{}, fmt.Errorf("validate workload-storage backup before mutation: %w", err)
		}
		keys := make([]string, 0, len(workloadStorage))
		for key := range workloadStorage {
			keys = append(keys, key)
		}
		if err := validateRecoveredLogicalResources(recoveryManifest, applicationbackup.StateWorkloadStorage, keys); err != nil {
			return applicationRestoreData{}, err
		}
	}
	var logsHistory logsprovider.HistoryBackup
	if recoveryManifestHasSelected(recoveryManifest, applicationbackup.StateLogs) {
		var found bool
		logsHistory, found, err = applicationbackup.LogsHistoryFromPayload(payload)
		if err != nil {
			return applicationRestoreData{}, fmt.Errorf("validate log-history recovery payload before mutation: %w", err)
		}
		if !found {
			return applicationRestoreData{}, errors.New("recovery manifest selects observability.logs but the payload is missing")
		}
	}
	return applicationRestoreData{manifest: m, recoveryManifest: recoveryManifest, postgresBackups: postgresBackups, secretBackup: secretBackup, objectStorage: objectBackups, workloadStorage: workloadStorage, logsHistory: logsHistory}, nil
}

func recoveryManifestHasSelected(manifest applicationbackup.RecoveryManifest, class applicationbackup.RecoveryStateClass) bool {
	for _, contributor := range manifest.Contributors {
		if contributor.StateClass == class && contributor.Selected {
			return true
		}
	}
	return false
}

func bucketBackupNames(backups []objectstorage.BucketBackup) []string {
	names := make([]string, 0, len(backups))
	for _, backup := range backups {
		names = append(names, backup.LogicalBucket)
	}
	return names
}

func validateRecoveredLogicalResources(manifest applicationbackup.RecoveryManifest, class applicationbackup.RecoveryStateClass, actual []string) error {
	expected := map[string]struct{}{}
	for _, contributor := range manifest.Contributors {
		if contributor.StateClass == class && contributor.Selected && contributor.Support == applicationbackup.RecoverySupported {
			expected[contributor.LogicalResource] = struct{}{}
		}
	}
	got := map[string]struct{}{}
	for _, value := range actual {
		got[value] = struct{}{}
	}
	if len(expected) != len(got) {
		return fmt.Errorf("recovery payload for %s does not match recovery manifest", class)
	}
	for value := range expected {
		if _, ok := got[value]; !ok {
			return fmt.Errorf("recovery payload for %s is missing %q", class, value)
		}
	}
	return nil
}
