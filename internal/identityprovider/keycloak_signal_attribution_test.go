package identityprovider

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/observability"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// TestKeycloakSignalRegistrationTargetProductionAttribution is the producer-
// side regression w267 CHANGES 7196719214 item 1 demands: the record the
// ACTUAL producer-to-registry path (registerKeycloakSignals ->
// observability.RegisterProviderSignals) writes must itself carry a
// collision-free, validated Target identity in its canonical runtime://
// Target grammar — never the bare endpoint literal "keycloak:9000" that
// attributed the record to no Target. The attribution names the producer's
// own state root (the Target's identity), and the service half stays the
// descriptor endpoint with its port.
func TestKeycloakSignalRegistrationTargetProductionAttribution(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	stateRoot, err := deployment.TargetStateRoot("docker-dev")
	if err != nil || stateRoot == "" {
		t.Fatalf("resolve docker-dev state root: %v", err)
	}
	app := keycloakSignalTestApp(t, "alpha", true)
	project := bhruntime.TargetInstallationProject("docker-dev")
	r := &localKeycloakRealization{
		app:       app,
		namespace: "docker-dev",
		dataDir:   stateRoot,
		files:     KeycloakFiles{Project: project, ConsumerNetwork: "baseharbor-identity-core-baseharbor-docker-dev-core"},
	}
	if err := r.registerKeycloakSignals(); err != nil {
		t.Fatalf("registerKeycloakSignals: %v", err)
	}
	sources, err := observability.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	var found *observability.SignalSource
	for i := range sources {
		if sources[i].ID == "keycloak:"+project && sources[i].Provider == capability.ProviderKeycloak {
			found = &sources[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("producer registered no keycloak metrics record under %q", "keycloak:"+project)
	}
	if found.Target == "keycloak:9000" || !strings.HasPrefix(found.Target, "runtime://") {
		t.Fatalf("production record must carry the canonical runtime:// attribution grammar, got %q", found.Target)
	}
	project2, service, ok := observability.ParseRuntimeTarget(found.Target)
	if !ok {
		t.Fatalf("production record target %q is not parseable in the canonical grammar", found.Target)
	}
	if project2 != project {
		t.Fatalf("attributed project = %q, want the installation project %q", project2, project)
	}
	if !strings.HasPrefix(service, "keycloak@") || !strings.HasSuffix(service, ":9000") {
		t.Fatalf("attributed service %q must name the keycloak endpoint with the state-root attribution", service)
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(service, "keycloak@"), ":9000")
	decoded, err := url.PathUnescape(encoded)
	if err != nil {
		t.Fatalf("attribution token %q is not URI-encoded state root: %v", encoded, err)
	}
	if filepath.Clean(decoded) != filepath.Clean(stateRoot) {
		t.Fatalf("attribution root = %q, want the producer's state root %q", decoded, stateRoot)
	}
}

// TestKeycloakSignalRegistrationDottedCollisionProductionShape is the dotted
// collision class (w267 CHANGES 7194027071 / 7196719214) at the PRODUCER:
// the two valid Targets "a.b" and "a-b" collapse to ONE installation project
// and ONE consumer network (the grammars map dots to hyphens), yet their
// production records — written through the real producer seam — must carry
// distinct Target attributions: each names its OWN state root.
func TestKeycloakSignalRegistrationDottedCollisionProductionShape(t *testing.T) {
	t.Setenv("BASEHARBOR_STATE_DIR", t.TempDir())
	dottedRoot, err := deployment.TargetStateRoot("a.b")
	if err != nil {
		t.Fatalf("a.b root: %v", err)
	}
	hyphenRoot, err := deployment.TargetStateRoot("a-b")
	if err != nil {
		t.Fatalf("a-b root: %v", err)
	}
	if filepath.Clean(dottedRoot) == filepath.Clean(hyphenRoot) {
		t.Fatal("the two valid Target roots must be distinct")
	}
	// The collision premise: the normalized project grammars coincide.
	if bhruntime.TargetInstallationProject("a.b") != bhruntime.TargetInstallationProject("a-b") {
		t.Fatal("test premise wrong: the installation projects already separate a.b from a-b")
	}
	app := keycloakSignalTestApp(t, "alpha", true)

	register := func(target, root string) observability.SignalSource {
		t.Helper()
		project := bhruntime.TargetInstallationProject(target)
		r := &localKeycloakRealization{
			app:       app,
			namespace: target,
			dataDir:   root,
			files:     KeycloakFiles{Project: project, ConsumerNetwork: "baseharbor-identity-core-baseharbor-a-b-core"},
		}
		if err := r.registerKeycloakSignals(); err != nil {
			t.Fatalf("register %s: %v", target, err)
		}
		sources, err := observability.ListAll()
		if err != nil {
			t.Fatalf("ListAll: %v", err)
		}
		for _, source := range sources {
			if source.ID == "keycloak:"+project && source.Provider == capability.ProviderKeycloak && source.OwnerApplication == app.Name {
				return source
			}
		}
		t.Fatalf("no record registered for %s", target)
		return observability.SignalSource{}
	}

	dotted := register("a.b", dottedRoot)
	hyphen := register("a-b", hyphenRoot)
	if dotted.Target == hyphen.Target {
		t.Fatalf("the two production records must attribute differently, both %q", dotted.Target)
	}
	if dotted.ID != hyphen.ID || dotted.Network != hyphen.Network {
		t.Fatalf("premise: the colliding names share ID grammar and network; got %q/%q %q/%q", dotted.ID, hyphen.ID, dotted.Network, hyphen.Network)
	}
}
