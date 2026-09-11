package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
)

func TestParseGuidedBackupArgs(t *testing.T) {
	name, output, err := parseGuidedBackupArgs([]string{"mailflow", "--output", "safe.bhbackup"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "mailflow" || output != "safe.bhbackup" {
		t.Fatalf("unexpected parse result name=%q output=%q", name, output)
	}
}

func TestParseGuidedRestoreArgs(t *testing.T) {
	backup, name, err := parseGuidedRestoreArgs([]string{"safe.bhbackup", "mailflow"})
	if err != nil {
		t.Fatal(err)
	}
	if backup != "safe.bhbackup" || name != "mailflow" {
		t.Fatalf("unexpected parse result backup=%q name=%q", backup, name)
	}
}

func TestGuidedParsersRejectPasswordArgument(t *testing.T) {
	if _, _, err := parseGuidedBackupArgs([]string{"--password", "secret-value"}); err == nil {
		t.Fatal("guided backup accepted a password-like argv option")
	}
	if _, _, err := parseGuidedRestoreArgs([]string{"safe.bhbackup", "--password", "secret-value"}); err == nil {
		t.Fatal("guided restore accepted a password-like argv option")
	}
}

func TestFormatBackupPreviewContainsDurableStateWithoutSecretNames(t *testing.T) {
	m := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "mailflow",
		Environment: "production",
		Services: application.Services{
			Postgres: true,
			Secrets:  true,
			PostgresInstances: map[string]application.ServiceInstance{
				"primary": {},
			},
		},
		Secrets: application.SecretRequirements{Required: []application.SecretRequirement{{Name: "API_TOKEN"}}},
	}
	var out bytes.Buffer
	formatBackupPreview(&out, m, "mailflow-production.bhbackup")
	text := out.String()
	for _, wanted := range []string{"Application: mailflow", "Environment: production", "PostgreSQL: primary", "Managed secrets: included", "never placed in argv"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("backup preview missing %q:\n%s", wanted, text)
		}
	}
	if strings.Contains(text, "API_TOKEN") {
		t.Fatalf("backup preview exposed secret name:\n%s", text)
	}
}

func TestFormatRestorePreviewExplainsImpactAndVerification(t *testing.T) {
	m := application.Manifest{
		Version:     application.CurrentVersion,
		Name:        "mailflow",
		Environment: "production",
		Services: application.Services{
			Postgres: true,
			Secrets:  true,
			PostgresInstances: map[string]application.ServiceInstance{
				"primary": {},
			},
		},
	}
	createdAt := time.Date(2026, 9, 11, 10, 30, 0, 0, time.UTC)
	entries := []applicationbackup.Entry{
		{Name: "metadata/application.json", Kind: "metadata"},
		{Name: "postgres/primary.dump", Kind: "postgres"},
		{Name: "secrets/openbao.json", Kind: "secrets"},
	}
	var out bytes.Buffer
	formatRestorePreview(&out, "mailflow.bhbackup", m, createdAt, entries)
	text := out.String()
	for _, wanted := range []string{"Restore preview", "Application: mailflow", "Environment: production", "Created: 2026-09-11T10:30:00Z", "PostgreSQL: primary", "Managed secrets: included", "stopped/recreated", "readiness must pass"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("restore preview missing %q:\n%s", wanted, text)
		}
	}
}

func TestPromptGuidedConfirmationDefaultsAreSafe(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input      string
		defaultYes bool
		want       bool
	}{
		{name: "backup default yes", input: "\n", defaultYes: true, want: true},
		{name: "restore default no", input: "\n", defaultYes: false, want: false},
		{name: "explicit yes", input: "yes\n", defaultYes: false, want: true},
		{name: "explicit no", input: "no\n", defaultYes: true, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "answer")
			if err := os.WriteFile(path, []byte(tc.input), 0o600); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			var out bytes.Buffer
			got, err := promptGuidedConfirmation(file, &out, "Continue?", tc.defaultYes)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("confirmation=%v want %v", got, tc.want)
			}
		})
	}
}

func TestWithInMemoryPasswordFileUsesOwnerOnlyNonDiskFile(t *testing.T) {
	password := []byte("correct horse battery staple")
	err := withInMemoryPasswordFile(password, func(path string) error {
		if !strings.HasPrefix(path, "/proc/self/fd/") {
			t.Fatalf("password path %q is not an in-memory fd path", path)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("password memfd mode=%o want 600", info.Mode().Perm())
		}
		for readNumber := 1; readNumber <= 2; readNumber++ {
			got, err := readBackupPasswordFile(path)
			if err != nil {
				t.Fatalf("password memfd read %d: %v", readNumber, err)
			}
			if !bytes.Equal(got, password) {
				zeroBytes(got)
				t.Fatalf("password memfd content mismatch on read %d", readNumber)
			}
			zeroBytes(got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
