package hosttrust

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordedStatusPreservesForeignAndMissingAnchors(t *testing.T) {
	root := t.TempDir()
	foreign := filepath.Join(root, "foreign.crt")
	if err := os.WriteFile(foreign, []byte("operator certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.crt")
	if err := os.Symlink(foreign, link); err != nil {
		t.Fatal(err)
	}
	state := State{Version: stateVersion, Anchors: []AnchorRecord{{Fingerprint: "foreign", Path: foreign}, {Fingerprint: "symlink", Path: link}, {Fingerprint: "missing", Path: filepath.Join(root, "gone.crt")}}}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(root, "host-trust.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	statuses, err := InspectRecorded(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 3 || statuses[0].State != "unverified" || statuses[1].State != "unverified" || statuses[2].State != "missing" {
		t.Fatalf("incorrect observations: %+v", statuses)
	}
	after, err := os.ReadFile(foreign)
	if err != nil || string(after) != "operator certificate" {
		t.Fatal("foreign certificate modified")
	}
}

func TestRecordedStatusVerifiesRetainedCertificate(t *testing.T) {
	root := t.TempDir()
	ca := testCA(t, "retained CA")
	_, fingerprint, err := ParseCA(ca)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "retained.crt")
	if err := os.WriteFile(path, ca, 0600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(State{Version: stateVersion, Anchors: []AnchorRecord{{Fingerprint: fingerprint, Backend: "fake", Path: path}}})
	if err := os.WriteFile(filepath.Join(root, "host-trust.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	statuses, err := InspectRecorded(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].State != "verified" || statuses[0].Fingerprint != fingerprint {
		t.Fatalf("missing retained certificate: %+v", statuses)
	}
}
