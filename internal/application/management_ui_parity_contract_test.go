package application

import (
	"strings"
	"testing"
)

func TestNewProviderManagementUIIntentRoundTrip(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "ui-demo",
		Environment:   "dev",
		Services: Services{
			KeyValue:                     true,
			KeyValueManagementUI:         true,
			DocumentDatabase:             true,
			DocumentDatabaseManagementUI: true,
			MessagingPubSub:              true,
			MessagingManagementUI:        true,
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}

	yaml := m.YAML()
	for _, want := range []string{
		"  key_value:\n    enabled: true\n    management_ui: true\n",
		"  document_database:\n    enabled: true\n    management_ui: true\n",
		"  messaging_pubsub:\n    enabled: true\n    management_ui: true\n",
	} {
		if !strings.Contains(yaml, want) {
			t.Fatalf("management UI YAML missing %q:\n%s", want, yaml)
		}
	}

	got, err := ParseYAML(yaml)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Services.KeyValueManagementUI || !got.Services.DocumentDatabaseManagementUI || !got.Services.MessagingManagementUI {
		t.Fatalf("management UI intent did not survive round trip: %#v", got.Services)
	}
}

func TestNewProviderManagementUIIntentFailsClosed(t *testing.T) {
	cases := []Manifest{
		{Version: CurrentVersion, ApplicationID: MustNewApplicationID(), Name: "kv", Environment: "dev", Services: Services{SQL: true, KeyValueManagementUI: true}},
		{Version: CurrentVersion, ApplicationID: MustNewApplicationID(), Name: "doc", Environment: "dev", Services: Services{SQL: true, DocumentDatabaseManagementUI: true}},
		{Version: CurrentVersion, ApplicationID: MustNewApplicationID(), Name: "msg", Environment: "dev", Services: Services{SQL: true, MessagingManagementUI: true}},
	}
	for _, m := range cases {
		if err := m.Validate(); err == nil {
			t.Fatalf("expected management UI dependency validation failure for %#v", m.Services)
		}
	}
}

func TestMessagingManagementUIRendersOnce(t *testing.T) {
	m := Manifest{
		Version:       CurrentVersion,
		ApplicationID: MustNewApplicationID(),
		Name:          "msg",
		Environment:   "dev",
		Services: Services{
			MessagingQueue:        true,
			MessagingPubSub:       true,
			MessagingStream:       true,
			MessagingManagementUI: true,
		},
	}
	yaml := m.YAML()
	if got := strings.Count(yaml, "management_ui: true"); got != 1 {
		t.Fatalf("messaging management UI rendered %d times:\n%s", got, yaml)
	}
}
