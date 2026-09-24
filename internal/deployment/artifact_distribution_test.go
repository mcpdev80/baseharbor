package deployment

import "testing"

func TestArtifactDistributionStateRoundTrip(t *testing.T) {
	values := map[string]string{}
	want := ArtifactDistributionState{
		RepositoryPrefix: "ghcr.io/mcpdev80/baseharbor",
		BuildKitAddress:   "unix:///run/user/1001/buildkit/buildkitd.sock",
	}
	if err := ApplyArtifactDistributionState(values, want); err != nil {
		t.Fatal(err)
	}
	got, err := ArtifactDistributionStateFromValues(values)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("state = %#v, want %#v", got, want)
	}
}

func TestNormalizeArtifactRepositoryPrefixRejectsPortableContractLeakShapes(t *testing.T) {
	for _, value := range []string{
		"https://ghcr.io/mcpdev80/baseharbor",
		"ghcr.io/mcpdev80/baseharbor@sha256:deadbeef",
		"baseharbor",
	} {
		if _, err := NormalizeArtifactRepositoryPrefix(value); err == nil {
			t.Fatalf("repository prefix %q unexpectedly accepted", value)
		}
	}
}


func TestArtifactDestinationIsDeploymentDerived(t *testing.T) {
	got, err := ArtifactDestination(
		"ghcr.io/mcpdev80/baseharbor",
		"BaseHarbor Demo",
		"Dev",
		"demo_app",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "ghcr.io/mcpdev80/baseharbor/baseharbor-demo-demo-app:dev"
	if got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}
