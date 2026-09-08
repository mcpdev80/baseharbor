package applicationbackup

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestApplicationManifestPayloadEntryRoundTrip(t *testing.T) {
	m := application.WithPostgresInstances(application.New("mailflow", "dev", true, true, true), "primary", "analytics")
	entry, err := ApplicationManifestPayloadEntry(m)
	if err != nil {
		t.Fatal(err)
	}
	payload := Payload{Manifest: Manifest{SchemaVersion: SchemaVersion, Application: "mailflow", Environment: "dev"}, Entries: []PayloadEntry{entry}}
	restored, err := ApplicationManifestFromPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if restored.YAML() != m.YAML() {
		t.Fatalf("restored manifest differs:\n%s\nwant:\n%s", restored.YAML(), m.YAML())
	}
}

func TestApplicationManifestFromPayloadRejectsIdentityMismatch(t *testing.T) {
	m := application.New("mailflow", "dev", true, false, false)
	entry, err := ApplicationManifestPayloadEntry(m)
	if err != nil {
		t.Fatal(err)
	}
	payload := Payload{Manifest: Manifest{SchemaVersion: SchemaVersion, Application: "other", Environment: "dev"}, Entries: []PayloadEntry{entry}}
	_, err = ApplicationManifestFromPayload(payload)
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity rejection, got %v", err)
	}
}
