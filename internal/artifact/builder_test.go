package artifact

import (
	"context"
	"fmt"
	"testing"
)

type fakeBuilder struct {
	built []BuildRequest
}

func (b *fakeBuilder) Build(_ context.Context, request BuildRequest, destination string) (Artifact, error) {
	b.built = append(b.built, request)
	return Artifact{
		Service:   request.Service,
		Reference: destination + "@sha256:deadbeef",
		Digest:    "sha256:deadbeef",
	}, nil
}

func TestCompleteResolutionBuildsSourceRequestsAndPreservesExistingArtifacts(t *testing.T) {
	builder := &fakeBuilder{}
	got, err := CompleteResolution(
		context.Background(),
		Resolution{
			Artifacts: []Artifact{{Service: "worker", Reference: "example/worker@sha256:aaaa"}},
			Builds:    []BuildRequest{{Service: "demo-app", Context: "/repo/demo-app", Dockerfile: "Dockerfile"}},
		},
		BuildDestinations{"demo-app": "registry.example/demo:dev"},
		builder,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete() {
		t.Fatalf("completed resolution still has builds: %#v", got)
	}
	if len(builder.built) != 1 || builder.built[0].Service != "demo-app" {
		t.Fatalf("builder calls = %#v", builder.built)
	}
	images := got.Images()
	if images["worker"] != "example/worker@sha256:aaaa" {
		t.Fatalf("existing artifact lost: %#v", images)
	}
	if images["demo-app"] != "registry.example/demo:dev@sha256:deadbeef" {
		t.Fatalf("built artifact missing: %#v", images)
	}
}

func TestCompleteResolutionRequiresDestination(t *testing.T) {
	_, err := CompleteResolution(
		context.Background(),
		Resolution{Builds: []BuildRequest{{Service: "demo-app"}}},
		nil,
		&fakeBuilder{},
	)
	if err == nil {
		t.Fatal("missing artifact destination unexpectedly accepted")
	}
}

type badBuilder struct{}

func (badBuilder) Build(context.Context, BuildRequest, string) (Artifact, error) {
	return Artifact{}, fmt.Errorf("boom")
}

func TestCompleteResolutionPreservesBuilderFailure(t *testing.T) {
	_, err := CompleteResolution(
		context.Background(),
		Resolution{Builds: []BuildRequest{{Service: "demo-app"}}},
		BuildDestinations{"demo-app": "example/demo:dev"},
		badBuilder{},
	)
	if err == nil {
		t.Fatal("builder failure unexpectedly ignored")
	}
}
