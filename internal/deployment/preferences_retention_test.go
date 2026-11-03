package deployment

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/targetsession"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// TestReviewerRetainsPreferences is the exact reviewer-repro from review-1333e:
// seed a valid nonempty preference array, save the same deployment with nil
// preferences, reload, assert identical. This was the data-loss regression.
func TestReviewerRetainsPreferences(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record with preferences: %v", err)
	}

	// Same deployment, nil preferences (a lifecycle save that does not carry
	// preferences). The retained array must survive.
	nilPrefs := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(nilPrefs); err != nil {
		t.Fatalf("save with nil preferences: %v", err)
	}

	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("reload after nil-prefs save: %v", err)
	}
	var before, after []map[string]string
	if err := json.Unmarshal(seed.DesiredProviderPreferences, &before); err != nil {
		t.Fatalf("unmarshal seed prefs: %v", err)
	}
	if err := json.Unmarshal(got.DesiredProviderPreferences, &after); err != nil {
		t.Fatalf("unmarshal reloaded prefs: %v", err)
	}
	if len(after) != len(before) || after[0]["profile"] != before[0]["profile"] {
		t.Fatalf("preferences lost on nil save, seed=%v reloaded=%v", before, after)
	}
}

// TestNilRetentionDifferentOwnerRejected proves that a nil save against a
// previous record owned by a DIFFERENT deployment owner retains nothing and is
// refused, not silently overwritten.
func TestNilRetentionDifferentOwnerRejected(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	// Inject a different application/environment into the stored file to simulate
	// a previous record owned by a different deployment owner.
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Identity.ApplicationID = "22222222-2222-4222-8222-222222222222"
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	if err := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}); err == nil {
		t.Fatalf("nil save against different owner must be refused, got nil error")
	}
}

// TestExplicitEmptyArrayReplaced proves that an explicit empty array [] is
// treated as a replacement and overwrites previously retained preferences.
func TestExplicitEmptyArrayReplaced(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	// Explicit empty array is an explicit replacement.
	empty := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[]`),
	}
	if err := SaveDeploymentRecord(empty); err != nil {
		t.Fatalf("save explicit empty array: %v", err)
	}
	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	var after []map[string]string
	if err := json.Unmarshal(got.DesiredProviderPreferences, &after); err != nil {
		t.Fatalf("unmarshal reloaded prefs: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("explicit empty array must clear retained prefs, got %v", after)
	}
}

// TestInvalidPreferencesRejected proves that invalid (non-array JSON) desired
// provider preferences are rejected on save.
func TestInvalidPreferencesRejected(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	bad := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`{"not":"an array"`),
	}
	if err := SaveDeploymentRecord(bad); err == nil {
		t.Fatalf("invalid preferences must be rejected, got nil error")
	}
}

// TestForgedOwnerBypassStillHolds is the existing forged-owner regression kept
// intact: an external reconciler remains authoritative across a forged
// delegated update and a subsequent direct save is refused.
func TestForgedOwnerBypassStillHolds(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerExternal},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed external record: %v", err)
	}

	forged := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied: AppliedDeployment{RuntimeProvider: "k8s", Delivery: delivery.Selection{
			Mode: delivery.ModeDelegated, Provider: "gitops/argocd", Owner: delivery.OwnerExternal, Placement: delivery.PlacementExternal,
		}},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: ""},
	}
	if err := SaveDeploymentRecord(forged); err != nil {
		t.Fatalf("delegated update must succeed: %v", err)
	}
	got, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Observed.Owner != delivery.OwnerExternal {
		t.Fatalf("persisted owner must stay canonical external, got %q", got.Observed.Owner)
	}
	direct := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(direct); !errors.Is(err, ErrUnauthorizedMutation) {
		t.Fatalf("direct apply after forged delegated update must be refused, got %v", err)
	}
}

// TestCallerConflictNotTrusted proves the caller-supplied Observed.Conflict is
// discarded: with previous.conflict=false and no persisted conflict, a same
// owner update is NOT rejected because of the caller conflict field alone. The
// semantics are previous-conflict OR store-own-revelation; caller-feld ignored.
func TestCallerConflictNotTrusted(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}

	// Same-owner update with a caller-supplied conflict flag but a persisted
	// previous.conflict=false. The guard must NOT reject on the caller field
	// alone: previous-conflict OR store-own-revelation governs, caller ignored.
	record := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor, Conflict: true},
	}
	if err := SaveDeploymentRecord(record); err != nil {
		t.Fatalf("same-owner update must not be rejected on caller-conflict alone, got %v", err)
	}
}

// TestPreviousConflictStillFailsClosed re-confirms that a persisted previous
// conflict is honored: same-owner update under a carried conflict refuses.
func TestPreviousConflictStillFailsClosed(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	seed := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Observed.Conflict = true
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("write injected: %v", err)
	}

	same := DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}
	if err := SaveDeploymentRecord(same); !errors.Is(err, ErrConflictingOneWriter) {
		t.Fatalf("same-owner update under carried conflict must refuse with ErrConflictingOneWriter, got %v", err)
	}
}

// TestRetentionPreviousInvalidSelectionFailsClosed proves that a nil save against
// a previous record that the full loader rejects (invalid delivery selection)
// fails closed: the save is refused and the previously owned preferences stay.
func TestRetentionPreviousInvalidSelectionFailsClosed(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	// Corrupt the persisted selection so the full loader rejects the previous
	// record on re-read (invalid delivery selection).
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Applied.Delivery = delivery.Selection{Mode: delivery.ModeDirect, Provider: "not a valid provider"}
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	// Nil save against the corrupt previous record must be refused, not silently
	// succeed and drop the retained preferences.
	if err := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}); err == nil {
		t.Fatalf("nil save against previous invalid-selection record must fail closed, got nil error")
	}
}

// TestRetentionPreviousInvalidRemoteProjectFailsClosed proves that a nil save
// against a previous record rejected by the full loader for an invalid
// remote-project binding fails closed.
func TestRetentionPreviousInvalidRemoteProjectFailsClosed(t *testing.T) {
	id := testDeploymentIdentity(t, "first", "app-a", "prod")
	stateDir := t.TempDir()
	t.Setenv("BASEHARBOR_STATE_DIR", stateDir)

	seed := DeploymentRecord{
		Version:                    DeploymentRecordVersion,
		Identity:                   id,
		Source:                     DeploymentSource{Kind: "repository"},
		Applied:                    AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed:                   ObservedDeployment{State: "applying", Ready: false, Owner: delivery.OwnerBaseHarbor},
		DesiredProviderPreferences: json.RawMessage(`[{"profile":"default"}]`),
	}
	if err := SaveDeploymentRecord(seed); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	// Inject a remote-project binding that does not match target/runtime so the
	// full loader rejects the previous record.
	path := filepath.Join(stateDir, "targets", id.Target, "deployments", id.DeploymentID, "deployment.json")
	record, err := LoadDeploymentRecord(id)
	if err != nil {
		t.Fatalf("load seed: %v", err)
	}
	record.Applied.RemoteProject = &targetsession.ProjectRecord{
		Scope: targetenrollment.Scope{
			TargetID: "other-target",
			Runtime:  "k8s",
		},
	}
	blob, _ := json.MarshalIndent(record, "", "  ")
	blob = append(blob, '\n')
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("rewrite seed: %v", err)
	}

	if err := SaveDeploymentRecord(DeploymentRecord{
		Version:  DeploymentRecordVersion,
		Identity: id,
		Source:   DeploymentSource{Kind: "repository"},
		Applied:  AppliedDeployment{RuntimeProvider: "docker", Delivery: delivery.Direct()},
		Observed: ObservedDeployment{State: "ready", Ready: true, Owner: delivery.OwnerBaseHarbor},
	}); err == nil {
		t.Fatalf("nil save against previous invalid-remote-project record must fail closed, got nil error")
	}
}
