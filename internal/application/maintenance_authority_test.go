package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// maintenanceCandidateDigest is a content-addressed candidate binding.
func maintenanceCandidateDigest(label string) string {
	sum := sha256.Sum256([]byte(label))
	return "sha256:" + hex.EncodeToString(sum[:])
}

const (
	maintenanceTestApplicationID = "11111111-1111-4111-8111-111111111111"
	maintenanceTestDeploymentID  = "22222222-2222-4222-8222-222222222222"
)

func maintenanceTestRecord(operation, archive string, revision int) MaintenanceAuthorityRecord {
	return MaintenanceAuthorityRecord{
		ContractVersion: MaintenanceAuthorityContractVersion,
		ApplicationID:   maintenanceTestApplicationID,
		DeploymentID:    maintenanceTestDeploymentID,
		Target:          "managed", Environment: "dev",
		Operation:      operation,
		Revision:       revision,
		Digest:         MaintenanceDigest("managed", "dev", operation, revision, "", archive),
		ArchiveBinding: archive,
	}
}

func maintenanceProtectedState(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "deployment")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestMaintenanceAuthorityRecordValidation(t *testing.T) {
	valid := maintenanceTestRecord("backup", "sha256:unit", 1)
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	// An empty window is legal to bind but is never a durable operation.
	if err := (MaintenanceAuthorityRecord{}).Validate(); err == nil {
		t.Fatal("an empty record validated as a durable window")
	}
	for name, mutate := range map[string]func(*MaintenanceAuthorityRecord){
		"contract":     func(r *MaintenanceAuthorityRecord) { r.ContractVersion = "v0" },
		"application":  func(r *MaintenanceAuthorityRecord) { r.ApplicationID = "not-a-uuid" },
		"deployment":   func(r *MaintenanceAuthorityRecord) { r.DeploymentID = strings.Repeat("0", 36) },
		"target":       func(r *MaintenanceAuthorityRecord) { r.Target = " " },
		"environment":  func(r *MaintenanceAuthorityRecord) { r.Environment = "" },
		"operation":    func(r *MaintenanceAuthorityRecord) { r.Operation = "replay" },
		"digest":       func(r *MaintenanceAuthorityRecord) { r.Digest = "sha256:truncated" },
		"revision":     func(r *MaintenanceAuthorityRecord) { r.Revision = -1 },
		"zeroRevision": func(r *MaintenanceAuthorityRecord) { r.Revision = 0 },
	} {
		broken := valid
		mutate(&broken)
		if err := broken.Validate(); err == nil {
			t.Fatalf("%s was accepted in a durable maintenance window", name)
		}
	}
}

func TestMaintenanceAuthorityCommitAndReload(t *testing.T) {
	root := maintenanceProtectedState(t)
	record := maintenanceTestRecord("backup", "sha256:unit", 1)
	committed, isNew, err := CommitMaintenanceAuthority(root, record)
	if err != nil || !isNew {
		t.Fatalf("first commit: isNew=%t err=%v", isNew, err)
	}
	loaded, found, err := LoadMaintenanceAuthority(root)
	if err != nil || !found {
		t.Fatalf("the durable window was not readable: %v", err)
	}
	if loaded.ApplicationID != committed.ApplicationID || loaded.Digest != committed.Digest {
		t.Fatalf("reload differs from commit: %+v", loaded)
	}
	// Re-affirming the identical binding is the same durable operation.
	again, isNew, err := CommitMaintenanceAuthority(root, record)
	if err != nil || isNew || again.Revision != committed.Revision {
		t.Fatalf("a replay was not idempotent: isNew=%t %+v %v", isNew, again, err)
	}
}

func TestMaintenanceAuthorityRejectsUnownedAndForeignState(t *testing.T) {
	record := maintenanceTestRecord("backup", "sha256:unit", 1)
	// A relative or shared state root is never written to.
	if _, isNew, err := CommitMaintenanceAuthority("relative/deployment", record); err == nil || isNew {
		t.Fatalf("a relative state root was written: %v", err)
	}
	shared := t.TempDir()
	if err := os.Chmod(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, isNew, err := CommitMaintenanceAuthority(shared, record); err == nil || isNew {
		t.Fatalf("a shared state root was written: %v", err)
	}
	// A symlinked state path is refused before anything is created.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(shared, link); err == nil {
		if _, isNew, err := CommitMaintenanceAuthority(link, record); err == nil || isNew {
			t.Fatalf("a symlinked state root was written: %v", err)
		}
	}
}

func TestMaintenanceAuthorityNeverRegressesOrSkipsRevision(t *testing.T) {
	root := maintenanceProtectedState(t)
	first := maintenanceTestRecord("backup", "sha256:one", 1)
	if _, _, err := CommitMaintenanceAuthority(root, first); err != nil {
		t.Fatal(err)
	}
	// A skipped revision cannot be committed: the counter advances by one.
	skipped := maintenanceTestRecord("restore", "", 7)
	if _, isNew, err := CommitMaintenanceAuthority(root, skipped); err == nil || isNew {
		t.Fatalf("revision 7 was committed over revision 1: %v", err)
	}
	// The next revision is accepted and records its own operation.
	next := maintenanceTestRecord("restore", "", 2)
	if _, isNew, err := CommitMaintenanceAuthority(root, next); err != nil || !isNew {
		t.Fatalf("the next revision was refused: %v", err)
	}
	// Replaying the first operation is idempotent from the durable history and
	// must not rewrite it.
	firstAgain, isNew, err := CommitMaintenanceAuthority(root, first)
	if err != nil || isNew {
		t.Fatalf("an old binding was not recognised as idempotent: %v", err)
	}
	if firstAgain.Revision != first.Revision || firstAgain.Operation != "backup" {
		t.Fatalf("the reported durable operation is not the recorded one: %+v", firstAgain)
	}
	// A foreign deployment's record is refused at either location.
	foreign := first
	foreign.DeploymentID = "33333333-3333-4333-8333-333333333333"
	foreign.Digest = MaintenanceDigest("managed", "dev", "backup", 1, "", foreign.ArchiveBinding)
	if _, _, err := CommitMaintenanceAuthority(root, foreign); !errors.Is(err, ErrMaintenanceAuthorityForeign) {
		t.Fatalf("a foreign window record was accepted: %v", err)
	}
	// An old record with no identity is not a window: it is never adopted.
	empty := MaintenanceAuthorityRecord{}
	if _, _, err := CommitMaintenanceAuthority(root, empty); err == nil {
		t.Fatal("an empty record was committed as a durable window")
	}
}

func TestMaintenanceAuthorityBindingIsContentSpecific(t *testing.T) {
	root := maintenanceProtectedState(t)
	first := maintenanceTestRecord("backup", "sha256:one", 1)
	if _, _, err := CommitMaintenanceAuthority(root, first); err != nil {
		t.Fatal(err)
	}
	// The same revision with different content is a different durable operation and
	// must be refused, not silently overwritten.
	changed := maintenanceTestRecord("backup", "sha256:two", 1)
	_, _, err := CommitMaintenanceAuthority(root, changed)
	if err == nil {
		t.Fatal("an existing revision was overwritten with a different binding")
	}
	if !strings.Contains(err.Error(), "revision") {
		t.Fatalf("the refusal does not identify the revision conflict: %v", err)
	}
	loaded, found, err := LoadMaintenanceAuthority(root)
	if err != nil || !found || loaded.ArchiveBinding != "sha256:one" {
		t.Fatalf("the retained binding changed: %+v %v", loaded, err)
	}
}

func TestMaintenanceAuthorityRejectsCorruptProtectedRecords(t *testing.T) {
	root := maintenanceProtectedState(t)
	window := filepath.Join(root, MaintenanceAuthorityFileName)
	for name, payload := range map[string]string{
		"truncated-json": "{",
		"unknown-field":  `{"contract_version":"` + MaintenanceAuthorityContractVersion + `","application_id":"` + maintenanceTestApplicationID + `","deployment_id":"` + maintenanceTestDeploymentID + `","target":"managed","environment":"dev","operation":"backup","revision":1,"digest":"sha256:` + strings.Repeat("a", 64) + `","surprise":true}`,
		"trailing-data":  `{"contract_version":"` + MaintenanceAuthorityContractVersion + `","application_id":"` + maintenanceTestApplicationID + `","deployment_id":"` + maintenanceTestDeploymentID + `","target":"managed","environment":"dev","operation":"backup","revision":1,"digest":"sha256:` + strings.Repeat("a", 64) + `"} {}`,
		"empty-object":   `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(window, []byte(payload), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadMaintenanceAuthority(root); err == nil {
				t.Fatalf("a %s maintenance window was accepted", name)
			}
			_, found, err := LoadMaintenanceAuthority(root)
			if err == nil && found {
				t.Fatal("an invalid window was reported as durable without an error")
			}
		})
	}
}

func TestMaintenanceAuthorityResolveReportsDurableState(t *testing.T) {
	identity := MaintenanceAuthorityRecord{
		ApplicationID: maintenanceTestApplicationID, DeploymentID: maintenanceTestDeploymentID,
		Target: "managed", Environment: "dev",
	}
	view, err := ResolveMaintenanceAuthority(MaintenanceAuthorityRecord{}, identity)
	if err != nil {
		t.Fatal(err)
	}
	if view.RecordExists || view.Revision != 0 {
		t.Fatalf("an unrecorded window reported durable state: %+v", view)
	}
	if view.ApplicationID != identity.ApplicationID || view.DeploymentID != identity.DeploymentID {
		t.Fatalf("an unrecorded window lost the deployment identity: %+v", view)
	}
	if view.OperationRevision() != 1 {
		t.Fatalf("the first operation must claim revision one: %+v", view)
	}
	retained := maintenanceTestRecord("backup", "sha256:unit", 4)
	view, err = ResolveMaintenanceAuthority(retained, identity)
	if err != nil {
		t.Fatal(err)
	}
	if !view.RecordExists || view.Revision != 4 || view.OperationRevision() != 5 {
		t.Fatalf("the retained window revision is not reported: %+v", view)
	}
	// An invalid retained record is never resolved into a usable view.
	corrupt := retained
	corrupt.Operation = "replay"
	if _, err := ResolveMaintenanceAuthority(corrupt, identity); err == nil {
		t.Fatal("an invalid retained window resolved successfully")
	}
}

func TestMaintenanceDigestAndOperationsAreBounded(t *testing.T) {
	base := MaintenanceDigest("managed", "dev", "backup", 1, "sha256:candidate", "sha256:unit")
	if !strings.HasPrefix(base, "sha256:") || len(base) != len("sha256:")+64 {
		t.Fatalf("digest is not a full sha256 binding: %q", base)
	}
	if base != MaintenanceDigest("managed", "dev", "backup", 1, "sha256:candidate", "sha256:unit") {
		t.Fatal("the same inputs produced different bindings")
	}
	for _, mutate := range []func(*[]string){
		func(a *[]string) { (*a)[0] = "other-target" },
		func(a *[]string) { (*a)[1] = "prod" },
		func(a *[]string) { (*a)[2] = "restore" },
		func(a *[]string) { (*a)[3] = "2" },
	} {
		args := []string{"managed", "dev", "backup", "1", "sha256:candidate", "sha256:unit"}
		mutate(&args)
		if MaintenanceDigest(args[0], args[1], args[2], mustAtoi(args[3]), args[4], args[5]) == base {
			t.Fatalf("the binding ignores the %q dimension", args[0]+"|"+args[1]+"|"+args[2]+"|"+args[3])
		}
	}
	// An unknown dimension is rejected rather than hashed into a false binding.
	if err := ValidateMaintenanceDigest("sha512:" + strings.Repeat("a", 128)); err == nil {
		t.Fatal("a non-sha256 binding was accepted")
	}
	if err := ValidateMaintenanceDigest("sha256:" + strings.Repeat("a", 63)); err == nil {
		t.Fatal("a short binding was accepted")
	}
	if !MaintenanceDigestMatches(base, base) || MaintenanceDigestMatches(base, base[:len(base)-1]+"b") {
		t.Fatal("binding comparison is wrong")
	}
	if len(MaintenanceOperations) != 4 {
		t.Fatalf("the maintenance operation set changed size: %v", MaintenanceOperations)
	}
	for _, operation := range MaintenanceOperations {
		if !IsMaintenanceOperation(operation) {
			t.Fatalf("%q is not reported as supported", operation)
		}
	}
	if IsMaintenanceOperation("replay") || IsMaintenanceOperation("") {
		t.Fatal("an unsupported operation was accepted by discovery")
	}
}

func mustAtoi(value string) int {
	var out int
	for _, r := range value {
		out = out*10 + int(r-'0')
	}
	return out
}

// TestMaintenanceWindowRecordIsSecretFree proves the durable record only ever
// carries digests and identity, never a path or secret material.
func TestMaintenanceWindowRecordIsSecretFree(t *testing.T) {
	root := maintenanceProtectedState(t)
	record := maintenanceTestRecord("backup", "sha256:"+strings.Repeat("c", 64), 1)
	if _, _, err := CommitMaintenanceAuthority(root, record); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, MaintenanceAuthorityFileName))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"archive_path", "path", "password", "secret", root, "/tmp/"} {
		if strings.Contains(strings.ToLower(string(data)), strings.ToLower(forbidden)) {
			t.Fatalf("the durable window carries %q: %s", forbidden, data)
		}
	}
	if _, ok := decoded["contract_version"]; !ok {
		t.Fatal("the durable window is not versioned")
	}
}

// TestMaintenanceWindowCandidateBindsTheExactSubmission pins the candidate
// dimension the client binds so a replaced candidate changes the binding.
func TestMaintenanceWindowCandidateBindsTheExactSubmission(t *testing.T) {
	record := deployment.DeploymentRecord{}
	if MaintenanceWindowCandidate(record) != "" {
		t.Fatal("a record without consent reported a candidate")
	}
	filled := record
	filled.Consent = &delivery.CandidateConsent{Digest: maintenanceCandidateDigest("candidate")}
	if MaintenanceWindowCandidate(filled) != maintenanceCandidateDigest("candidate") {
		t.Fatal("the retained candidate digest is not reported")
	}
	one := MaintenanceDigest("managed", "dev", "backup", 1, MaintenanceWindowCandidate(filled), "sha256:unit")
	filled.Consent = &delivery.CandidateConsent{Digest: maintenanceCandidateDigest("replaced")}
	other := MaintenanceDigest("managed", "dev", "backup", 1, MaintenanceWindowCandidate(filled), "sha256:unit")
	if one == other {
		t.Fatal("replacing the candidate left the maintenance binding unchanged")
	}
}
