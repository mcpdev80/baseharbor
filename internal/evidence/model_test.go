package evidence

import (
	"context"
	"testing"
	"time"
)

func TestSealIsStableForEquivalentRecordOrder(t *testing.T) {
	a := Bundle{
		Target: "local", Application: "demo", Environment: "dev",
		Desired: []Record{
			{Kind: StateDesired, ID: "b", Resource: "b"},
			{Kind: StateDesired, ID: "a", Resource: "a"},
		},
	}
	b := Bundle{
		Target: "local", Application: "demo", Environment: "dev",
		Desired: []Record{
			{Kind: StateDesired, ID: "a", Resource: "a"},
			{Kind: StateDesired, ID: "b", Resource: "b"},
		},
	}
	sealedA, err := Seal(a)
	if err != nil {
		t.Fatal(err)
	}
	sealedB, err := Seal(b)
	if err != nil {
		t.Fatal(err)
	}
	if sealedA.Integrity.Algorithm != "sha256" || sealedA.Integrity.Digest == "" {
		t.Fatalf("unexpected integrity: %#v", sealedA.Integrity)
	}
	if sealedA.Integrity.Digest != sealedB.Integrity.Digest {
		t.Fatalf("equivalent evidence produced different digests: %s != %s", sealedA.Integrity.Digest, sealedB.Integrity.Digest)
	}
}

func TestActorContextAndAuditEvent(t *testing.T) {
	ctx := WithActor(context.Background(), "mcp", "local-agent")
	event := NewAuditEvent(ctx, "local", "demo", "dev", "apply", "success")
	event.Timestamp = time.Date(2026, 9, 26, 20, 0, 0, 0, time.UTC)
	event.ID = auditEventID(event)
	if event.Actor.Interface != "mcp" || event.Actor.Identity != "local-agent" {
		t.Fatalf("unexpected actor: %#v", event.Actor)
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
}
