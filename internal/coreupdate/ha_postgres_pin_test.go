package coreupdate

import "testing"

func TestHAPostgresPinFailsClosedOnDrift(t *testing.T) {
	pin := BackingPin{Role: "core-ha-postgresql", Version: "18-spilo-4.1-p2", Image: "ghcr.io/zalando/spilo-18:4.1-p2", Digest: digestB}
	base := Realization{Kind: SQL, Installation: "core", Scope: "shared", Instance: "postgres-member-1", Owner: "baseharbor", Image: pin.Image, Digest: pin.Digest, Version: "4.1-p2"}
	same := ClassifyHAPostgresPin(base, pin)
	if same.Classification != NoChange || same.Installed.Version != pin.Version {
		t.Fatalf("identical immutable HA image cannot require restart: %+v", same)
	}
	tests := []struct {
		name  string
		alter func(*Realization)
	}{
		{"digest_drift", func(r *Realization) { r.Digest = digestA }},
		{"missing_digest", func(r *Realization) { r.Digest = "" }},
		{"changed_image", func(r *Realization) { r.Image += "-other" }},
		{"foreign_owner", func(r *Realization) { r.Owner = "external" }},
		{"wrong_kind", func(r *Realization) { r.Kind = Secrets }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.alter(&r)
			got := ClassifyHAPostgresPin(r, pin)
			if got.Classification != Unsupported || got.Reason == "" {
				t.Fatalf("unsafe HA PostgreSQL image accepted: %+v", got)
			}
		})
	}
}

func TestHAPostgresPinOnlyAdmitsPinnedPatchUpgrade(t *testing.T) {
	pin := BackingPin{Role: "core-ha-postgresql", Version: "18-spilo-4.1-p2", Image: "ghcr.io/zalando/spilo-18:4.1-p2", Digest: digestB}
	current := Realization{Kind: SQL, Installation: "core", Scope: "shared", Instance: "postgres-member-1", Owner: "baseharbor", Image: "ghcr.io/zalando/spilo-18:4.1-p1", Digest: digestA, Version: "4.1-p1"}
	delta := ClassifyHAPostgresPin(current, pin)
	if delta.Classification != BackupRequired || delta.Installed.Version != "18-spilo-4.1-p1" {
		t.Fatalf("owned same-major patch must require native backup/rolling: %+v", delta)
	}
	current.Digest = ""
	if got := ClassifyHAPostgresPin(current, pin); got.Classification != Unsupported {
		t.Fatalf("unpinned HA rolling mutation admitted: %+v", got)
	}
	current.Image = "ghcr.io/zalando/spilo-18:4.2-p1"
	current.Digest = digestA
	if got := ClassifyHAPostgresPin(current, pin); got.Classification != Unsupported {
		t.Fatalf("Spilo downgrade admitted: %+v", got)
	}
}
