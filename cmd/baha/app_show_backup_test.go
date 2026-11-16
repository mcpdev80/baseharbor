package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestFormatApplicationOverviewShowsLastBackupMetadata(t *testing.T) {
	overview := applicationOverview{
		Name:        "mailflow",
		Environment: "production",
		LastBackup: &application.BackupMetadata{
			Version:           application.LastBackupMetadataVersion,
			Application:       "mailflow",
			Environment:       "production",
			CreatedAt:         time.Date(2026, 9, 11, 0, 45, 0, 0, time.UTC),
			ArchivePath:       "/backups/mailflow-production.bhbackup",
			PostgresResources: []string{"primary", "analytics"},
			IncludesSecrets:   true,
		},
	}

	var out bytes.Buffer
	formatApplicationOverview(&out, overview)
	text := out.String()
	for _, wanted := range []string{
		"created              2026-09-11T00:45:00Z",
		"archive              /backups/mailflow-production.bhbackup",
		"PostgreSQL           primary, analytics",
		"managed secrets      included",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("overview output missing %q:\n%s", wanted, text)
		}
	}
	if strings.Contains(text, "SECRET_KEY") || strings.Contains(text, "postgres://") {
		t.Fatalf("backup overview exposed secret-bearing detail:\n%s", text)
	}
}
