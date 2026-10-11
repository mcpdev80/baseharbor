package application

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Gate-0 characterization: explicit intent must survive the v0.4.25 changes.
func TestV0425BaselineExplicitServices(t *testing.T) {
	for _, flags := range [][3]bool{
		{true, false, false}, {false, true, false}, {false, false, true},
		{true, true, false}, {true, false, true}, {false, true, true}, {true, true, true},
	} {
		m := New("baseline", "dev", flags[0], flags[1], flags[2])
		got, err := ParseYAML(m.YAML())
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, m) {
			t.Fatalf("explicit services %v changed on round trip: %#v", flags, got)
		}
	}
}

func TestV0425BaselineReadPreservesIdentityAndAuthoring(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, RepositoryManifestName)
	m := WithRequiredSecrets(New("baseline", "dev", true, true, true), "API_TOKEN")
	authored := "# developer-owned comment\n" + m.YAML()
	if err := os.WriteFile(path, []byte(authored), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		got, err := LoadManifestFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, m) {
			t.Fatalf("read changed canonical identity or intent: %#v", got)
		}
		if _, err := PortableContractFromManifest(got); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != authored {
		t.Fatal("inspection rewrote developer manifest")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("inspection materialized state")
	}
}

func TestV0425BaselineZeroCapabilityProjection(t *testing.T) {
	// Use explicit intent rather than New's legacy implicit SQL default.
	m := Manifest{Version: CurrentVersion, ApplicationID: MustNewApplicationID(), Name: "baseline", Environment: "dev"}
	m = WithWorkloadComponents(m, "app")
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Capabilities) != 0 || contract.Secrets.Managed || contract.Identity != nil {
		t.Fatalf("zero capability intent gained dependencies: %#v", contract)
	}
}
