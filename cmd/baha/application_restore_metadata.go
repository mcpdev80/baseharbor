package main

import (
	"context"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"time"
)

func recordVerifiedApplicationRestore(ctx context.Context, resolved resolvedApplication, data applicationRestoreData) error {
	m := data.manifest
	metadata := application.RecoveryMetadata{
		Version:       application.LastRecoveryMetadataVersion,
		ApplicationID: m.ApplicationID,
		DeploymentID:  resolved.DeploymentIdentity.DeploymentID,
		Application:   m.Name, Environment: m.Environment,
		RestoredAt: time.Now().UTC(), BackupCreatedAt: data.backupCreatedAt,
		ArchivePath: data.archivePath, Recovery: recoveryContributorMetadata(data.recoveryManifest, true),
	}
	if err := resolved.Store.RecordLastRecovery(metadata); err != nil {
		return fmt.Errorf("application was restored and verified but recording recovery metadata failed: %w", err)
	}
	return recordApplicationAudit(ctx, resolved, "restore", "success", "status+doctor verified", "recovery unit restored and application verification reached READY")
}
