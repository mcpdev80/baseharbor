package applicationbackup

import (
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestV0419RecoveryClassesAreAddressable(t *testing.T) {
	for _, class := range []RecoveryStateClass{
		StateDurableKeyValue,
		StateDocumentDatabase,
		StateMessagingQueue,
		StateMessagingPubSub,
		StateMessagingStream,
	} {
		got, err := ParseRecoveryStateClass(string(class))
		if err != nil {
			t.Fatalf("ParseRecoveryStateClass(%q): %v", class, err)
		}
		if got != class {
			t.Fatalf("ParseRecoveryStateClass(%q) = %q", class, got)
		}
	}
}

func TestMessagingRecoveryClassificationIsExplicitAndFailClosed(t *testing.T) {
	m := application.Manifest{
		Version:       application.CurrentVersion,
		ApplicationID: application.MustNewApplicationID(),
		Name:          "events",
		Environment:   "dev",
		Services: application.Services{
			MessagingQueue:           true,
			MessagingQueueInstances:  map[string]application.ServiceInstance{"jobs": {}},
			MessagingPubSub:          true,
			MessagingPubSubInstances: map[string]application.ServiceInstance{"events": {}},
			MessagingStream:          true,
			MessagingStreamInstances: map[string]application.ServiceInstance{"audit": {}},
		},
	}
	selection, err := DiscoverManifestRecovery(m)
	if err != nil {
		t.Fatal(err)
	}
	want := map[RecoveryStateClass]string{
		StateMessagingQueue:  "jobs",
		StateMessagingPubSub: "events",
		StateMessagingStream: "audit",
	}
	for class, logical := range want {
		found := false
		for _, contributor := range selection.Contributors {
			if contributor.StateClass != class {
				continue
			}
			found = true
			if contributor.LogicalResource != logical ||
				contributor.Support != RecoveryUnsupported ||
				!contributor.Durable ||
				contributor.Selected {
				t.Fatalf("%s recovery contributor = %#v", class, contributor)
			}
		}
		if !found {
			t.Fatalf("%s recovery contributor missing", class)
		}
	}
	if err := selection.ValidateForCapture(); err == nil {
		t.Fatal("complete recovery must fail closed while managed messaging recovery export is unsupported")
	}
	excluded, err := selection.Apply(nil, []RecoveryStateClass{
		StateMessagingQueue,
		StateMessagingPubSub,
		StateMessagingStream,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := excluded.ValidateForCapture(); err != nil {
		t.Fatalf("explicit partial recovery exclusion should be valid: %v", err)
	}
}
