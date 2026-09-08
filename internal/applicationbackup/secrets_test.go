package applicationbackup

import (
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/openbao"
)

func TestOpenBaoSecretPayloadRoundTrip(t *testing.T) {
	backup := openbao.ApplicationSecretBackup{
		Identity: openbao.ApplicationIdentity{Name: "mailflow", Environment: "dev"},
		Secrets: []openbao.ApplicationSecretBackupEntry{
			{Key: "API_TOKEN", Value: []byte("static-secret")},
			{Key: "dyn-0123456789abcdef0123456789abcdef", Value: []byte("dynamic-secret")},
		},
	}
	entry, err := OpenBaoPayloadEntry(backup)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := Build("mailflow", "dev", time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC), []PayloadEntry{entry}, []byte("correct horse battery staple"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := Open(archive, []byte("correct horse battery staple"))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := OpenBaoBackupFromPayload("mailflow", "dev", payload)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Identity != backup.Identity || len(restored.Secrets) != 2 {
		t.Fatalf("unexpected restored backup: %#v", restored)
	}
	if string(restored.Secrets[1].Value) != "dynamic-secret" {
		t.Fatalf("dynamic value = %q", restored.Secrets[1].Value)
	}
}

func TestOpenBaoBackupFromPayloadRejectsCrossApplicationRestore(t *testing.T) {
	backup := openbao.ApplicationSecretBackup{
		Identity: openbao.ApplicationIdentity{Name: "mailflow", Environment: "dev"},
		Secrets: []openbao.ApplicationSecretBackupEntry{{Key: "API_TOKEN", Value: []byte("secret")}},
	}
	entry, err := OpenBaoPayloadEntry(backup)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := Build("mailflow", "dev", time.Date(2026, time.September, 9, 0, 0, 0, 0, time.UTC), []PayloadEntry{entry}, []byte("password-123456"))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := Open(archive, []byte("password-123456"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = OpenBaoBackupFromPayload("other-app", "dev", payload)
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity rejection, got %v", err)
	}
}
