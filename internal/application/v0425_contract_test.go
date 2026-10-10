package application

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// This file is the cross-session semantic fixture handoff. Consumers run the
// same pure API rather than inventing separate CLI/JSON/MCP resolution policy.
func TestV0425PublishedResolutionFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/v0425-resolution.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name        string               `json:"name"`
		Manifest    string               `json:"manifest"`
		Snapshot    ResolutionSnapshot   `json:"snapshot"`
		Preferences []ProviderPreference `json:"preferences"`
		ErrorCode   string               `json:"error_code"`
		Core        CoreRequirement      `json:"core"`
		Existing    int                  `json:"existing"`
		Additional  int                  `json:"additional"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			m, err := ParseYAML(fixture.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(fixture.Snapshot)
			got, err := ResolveFootprint(m, fixture.Snapshot, fixture.Preferences)
			after, _ := json.Marshal(fixture.Snapshot)
			if string(before) != string(after) {
				t.Fatal("resolver mutated authoritative snapshot")
			}
			if fixture.ErrorCode != "" {
				var typed *ResolutionError
				if !errors.As(err, &typed) || typed.Code != fixture.ErrorCode {
					t.Fatalf("error=%v want %s", err, fixture.ErrorCode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Version != FootprintVersion || got.Core != fixture.Core || len(got.Existing) != fixture.Existing || len(got.Additional) != fixture.Additional {
				t.Fatalf("footprint=%#v", got)
			}
		})
	}
}

const contractAppID = "11111111-1111-4111-8111-111111111111"
const contractOtherID = "22222222-2222-4222-8222-222222222222"

func TestMinimalAuthoringUsesExistingGrammarWithoutAllocatingID(t *testing.T) {
	base := "version: 1\napp:\n  name: myapp\n  environment: dev\n"
	for _, services := range []string{"", "services:\n  sql: true\n", "services:\n  secrets: true\n", "services:\n  identity: true\n"} {
		input := base + services
		one, err := ParseYAML(input)
		if err != nil {
			t.Fatal(err)
		}
		two, err := ParseYAML(input)
		if err != nil {
			t.Fatal(err)
		}
		if one.ApplicationID != "" || !reflect.DeepEqual(one, two) {
			t.Fatal("parse allocated or changed identity")
		}
		if _, err := PortableContractFromManifest(one); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{"", "{}", base + "sql: true\n", base + "app:\n  id: bad\n", strings.Repeat("x", 70000)} {
		if _, err := ParseYAML(input); err == nil {
			t.Fatalf("accepted invalid or unsupported authoring %q", input[:min(len(input), 100)])
		}
	}
	m, err := ParseYAML(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(); err == nil {
		t.Fatal("empty intent claimed runnable workload")
	}
}

func TestIdentityConvergenceUsesExistingStoreAndPreservesRepository(t *testing.T) {
	root := t.TempDir()
	authored := "version: 1\napp:\n  name: myapp\n  environment: dev\n"
	path := filepath.Join(root, RepositoryManifestName)
	if err := os.WriteFile(path, []byte(authored), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifestFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m = WithWorkloadComponents(m, "app")
	store := Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	first, err := store.InitializeIdentity(m, IdentitySnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.InitializeIdentity(m, IdentitySnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if first.ApplicationID == "" || first.ApplicationID != second.ApplicationID {
		t.Fatal("identity changed")
	}
	read, err := ResolveIdentity(m, IdentitySnapshot{ApplicationIDs: []string{first.ApplicationID}, PreviouslyRegistered: true})
	if err != nil || read.ApplicationID != first.ApplicationID {
		t.Fatalf("read convergence: %#v %v", read, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != authored {
		t.Fatal("initialization changed authoring")
	}
	other := m
	other.ApplicationID = contractOtherID
	if _, err := store.InitializeIdentity(other, IdentitySnapshot{}); err == nil {
		t.Fatal("explicit identity replaced registered owner")
	}
}

func TestIdentityReadFailsClosedOnAmbiguousOrMissingOwner(t *testing.T) {
	m := Manifest{Version: 1, Name: "myapp", Environment: "dev"}
	if got, err := ResolveIdentity(m, IdentitySnapshot{}); err != nil || got.ApplicationID != "" {
		t.Fatal("fresh read allocated identity")
	}
	for _, snapshot := range []IdentitySnapshot{
		{ApplicationIDs: []string{contractAppID, contractOtherID}},
		{ApplicationIDs: []string{"invalid"}},
		{PreviouslyRegistered: true},
	} {
		if _, err := ResolveIdentity(m, snapshot); err == nil {
			t.Fatal("invalid identity owner accepted")
		}
	}
	m.ApplicationID = contractAppID
	got, err := ResolveIdentity(m, IdentitySnapshot{ApplicationIDs: []string{contractAppID, contractAppID}})
	if err != nil || got.ApplicationID != contractAppID {
		t.Fatal("existing explicit identity changed")
	}
}

func contractSnapshot() ResolutionSnapshot {
	r := capability.NewRegistry()
	_ = r.Register(capability.ProviderInstance{ID: "sql/shared", Provider: capability.PostgreSQL, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor})
	return ResolutionSnapshot{Target: "local", Registry: r, Facts: map[string]ProviderFact{
		"sql/shared": {Target: "local", State: ProviderRunning, Core: CoreRequired, Dependencies: []string{}},
	}}
}

func contractSQL() Manifest {
	return Manifest{Version: 1, ApplicationID: contractAppID, Name: "myapp", Environment: "dev", Services: Services{SQL: true}}
}

func TestFootprintBindingFixtures(t *testing.T) {
	req := capability.Requirement{Kind: capability.SQL, Name: "default"}
	fixtures := []struct {
		name     string
		change   func(*ResolutionSnapshot)
		explicit string
		code     string
		state    ProviderState
	}{
		{"automatic", nil, "", "", ProviderRunning},
		{"explicit", nil, "sql/shared", "", ProviderRunning},
		{"missing", nil, "absent", "missing", ""},
		{"incompatible", func(s *ResolutionSnapshot) { s.Registry.Instances[0].Provider = capability.Valkey }, "sql/shared", "incompatible", ""},
		{"ambiguous", func(s *ResolutionSnapshot) {
			s.Registry.Instances = append(s.Registry.Instances, capability.ProviderInstance{ID: "sql/other", Provider: capability.PostgreSQL, Scope: capability.ScopeApplication, Ownership: capability.OwnershipBaseHarbor, OwnerApplicationID: contractAppID, OwnerApplication: "myapp"})
		}, "", "ambiguous", ""},
		{"foreign", func(s *ResolutionSnapshot) {
			s.Registry.Instances[0].Scope = capability.ScopeApplication
			s.Registry.Instances[0].OwnerApplicationID = contractOtherID
			s.Registry.Instances[0].OwnerApplication = "otherapp"
		}, "sql/shared", "foreign", ""},
		{"observed-foreign", func(s *ResolutionSnapshot) {
			f := s.Facts["sql/shared"]
			f.State = ProviderForeign
			s.Facts["sql/shared"] = f
		}, "sql/shared", "foreign", ""},
		{"stopped-owned", func(s *ResolutionSnapshot) {
			f := s.Facts["sql/shared"]
			f.State = ProviderStoppedOwned
			s.Facts["sql/shared"] = f
		}, "", "", ProviderStoppedOwned},
		{"unverifiable", func(s *ResolutionSnapshot) { delete(s.Facts, "sql/shared") }, "", "", ProviderUnverifiable},
		{"newly-required", func(s *ResolutionSnapshot) {
			f := s.Facts["sql/shared"]
			f.State = ProviderNewRequired
			s.Facts["sql/shared"] = f
		}, "", "", ProviderNewRequired},
		{"foreign-target", func(s *ResolutionSnapshot) { f := s.Facts["sql/shared"]; f.Target = "other"; s.Facts["sql/shared"] = f }, "", "foreign-target", ""},
		{"external-byo", func(s *ResolutionSnapshot) {
			s.Registry.Instances[0].Scope = capability.ScopeExternal
			s.Registry.Instances[0].Ownership = capability.OwnershipExternal
			s.Registry.Instances[0].Reference = "operator-sql"
		}, "sql/shared", "", ProviderRunning},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			s := contractSnapshot()
			if fixture.change != nil {
				fixture.change(&s)
			}
			var prefs []ProviderPreference
			if fixture.explicit != "" {
				prefs = []ProviderPreference{{Requirement: req, InstanceID: fixture.explicit}}
			}
			before := s.Registry
			got, err := ResolveFootprint(contractSQL(), s, prefs)
			if fixture.code != "" {
				var typed *ResolutionError
				if !errors.As(err, &typed) || typed.Code != fixture.code {
					t.Fatalf("error=%v want %s", err, fixture.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, s.Registry) {
				t.Fatal("resolution mutated registry")
			}
			if len(got.Requirements) != 1 || got.Requirements[0].State != fixture.state {
				t.Fatalf("resolution=%#v", got)
			}
			if fixture.name == "external-byo" && got.Core != CoreNotRequired {
				t.Fatal("BYO required Core")
			}
			if fixture.state == ProviderNewRequired {
				if len(got.Additional) != 1 || len(got.Existing) != 0 {
					t.Fatal("incremental requirement misclassified")
				}
			} else if len(got.Existing) != 1 || len(got.Additional) != 0 {
				t.Fatal("existing provider misclassified")
			}
		})
	}
}

func TestFootprintCorelessAndUnboundAreDifferent(t *testing.T) {
	s := contractSnapshot()
	m := Manifest{Version: 1, Name: "myapp", Environment: "dev"}
	got, err := ResolveFootprint(m, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Core != CoreNotRequired || len(got.Existing) != 0 || len(got.Additional) != 0 || len(got.Unused) != 1 {
		t.Fatalf("coreless=%#v", got)
	}
	s.Registry = capability.NewRegistry()
	got, err = ResolveFootprint(contractSQL(), s, nil)
	if err != nil || got.Core != CoreUnknown || got.Requirements[0].Binding != "requested/unbound" {
		t.Fatalf("unbound=%#v %v", got, err)
	}
	if !reflect.DeepEqual(ManagementCoreRequirements(), []capability.Kind{capability.SQL, capability.Secrets, capability.Identity}) {
		t.Fatal("#810 weakened")
	}
}

func TestFootprintReusesBindingAndDependencyUnion(t *testing.T) {
	s := contractSnapshot()
	m := contractSQL()
	resource, err := capability.Resolve(m.Name, capability.Requirement{Kind: capability.SQL, Name: "default"}, capability.PostgreSQL)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{contractAppID, contractOtherID} {
		if err := s.Registry.BindDeployment(resource, id, "dev", "sql/shared"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Registry.Register(capability.ProviderInstance{ID: "secrets/shared", Provider: capability.OpenBao, Scope: capability.ScopeShared, Ownership: capability.OwnershipBaseHarbor}); err != nil {
		t.Fatal(err)
	}
	f := s.Facts["sql/shared"]
	f.Dependencies = []string{"secrets/shared", "secrets/shared"}
	s.Facts["sql/shared"] = f
	s.Facts["secrets/shared"] = ProviderFact{Target: "local", State: ProviderRunning, Core: CoreRequired, Dependencies: []string{}}
	got, err := ResolveFootprint(m, s, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Requirements[0].Binding != "bound" || len(got.Existing) != 2 {
		t.Fatalf("union=%#v", got)
	}
	for _, fp := range got.Existing {
		if fp.InstanceID == "sql/shared" && (!fp.Shared || fp.Consumers != 2) {
			t.Fatal("shared consumers lost")
		}
		if fp.MemoryBytes.Classification != "unknown" || fp.MemoryBytes.Value != nil || fp.Containers.Value != nil {
			t.Fatal("invented sizing")
		}
	}
	f.Dependencies = []string{"missing"}
	s.Facts["sql/shared"] = f
	if _, err := ResolveFootprint(m, s, nil); err == nil {
		t.Fatal("missing dependency accepted")
	}
	f.Dependencies = []string{"sql/shared"}
	s.Facts["sql/shared"] = f
	if _, err := ResolveFootprint(m, s, nil); err == nil {
		t.Fatal("dependency cycle accepted")
	}
}

func TestFootprintEvidenceValidation(t *testing.T) {
	n := int64(7)
	for _, q := range []Quantity{{Value: &n, Classification: "unknown"}, {Value: &n, Classification: "measured"}, {Classification: "estimated", Source: "adapter"}} {
		if err := q.Validate(); err == nil {
			t.Fatal("invalid evidence accepted")
		}
	}
	for _, class := range []string{"measured", "estimated"} {
		q := Quantity{Value: &n, Classification: class, Source: "runtime receipt"}
		if err := q.Validate(); err != nil {
			t.Fatal(err)
		}
		s := contractSnapshot()
		f := s.Facts["sql/shared"]
		f.MemoryBytes = q
		s.Facts["sql/shared"] = f
		got, err := ResolveFootprint(contractSQL(), s, nil)
		if err != nil || got.Existing[0].MemoryBytes.Classification != class {
			t.Fatalf("evidence lost: %v", err)
		}
	}
}
