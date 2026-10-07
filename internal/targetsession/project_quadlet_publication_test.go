package targetsession

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

type interruptedGraphTransport struct {
	*projectTestTransport
	failAt int
}

func (f *interruptedGraphTransport) Dispatch(ctx context.Context, scope targetenrollment.Scope, request Request) (Response, error) {
	response, err := f.projectTestTransport.Dispatch(ctx, scope, request)
	if len(f.calls) == f.failAt {
		return Response{}, errors.New("graph transport interrupted after admission")
	}
	return response, err
}

func TestQuadletGraphDoesNotStartBeforeEveryDependencyIsPublished(t *testing.T) {
	for _, failAt := range []int{0, 2, 3, 4, 5, 6} {
		t.Run(strconv.Itoa(failAt), func(t *testing.T) {
			transport := &interruptedGraphTransport{projectTestTransport: newProjectTestTransport(t, "podman"), failAt: failAt}
			runtime, err := NewProjectRuntime(transport, transport.scope)
			if err != nil {
				t.Fatal(err)
			}
			// The application sorts before its SQL dependency. Neither container
			// may start until both units and the network have been published.
			project, err := runtime.Stage(context.Background(), "owned", []ProjectFile{
				{Path: "a-app.container", Data: []byte("[Unit]\nRequires=z-sql.service\nAfter=z-sql.service\n[Container]\nImage=example/app:fixture\n")},
				{Path: "z-sql.container", Data: []byte("[Container]\nImage=example/sql:fixture\n")},
				{Path: "owned.network", Data: []byte("[Network]\nNetworkName=owned\n")},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = runtime.ApplyQuadletGraph(context.Background(), project, []string{"a-app.container", "z-sql.container", "owned.network"})
			if failAt == 0 && err != nil || failAt != 0 && !errors.Is(err, ErrUnavailable) {
				t.Fatal("graph interruption lost", err)
			}
			expectedCalls := 6 // One staging request, three publications, two activations.
			if failAt != 0 {
				expectedCalls = failAt
			}
			if len(transport.calls) != expectedCalls {
				t.Fatal("interrupted graph continued or replayed", len(transport.calls))
			}
			published := map[string]bool{}
			var activated []string
			for _, call := range transport.calls[1:] {
				var payload struct {
					Name   string `json:"name"`
					Enable bool   `json:"enable"`
				}
				if err := json.Unmarshal(call.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Enable && (!published["a-app.container"] || !published["z-sql.container"] || !published["owned.network"]) {
					t.Fatal("container started before complete graph publication")
				}
				if !payload.Enable {
					published[payload.Name] = true
				} else {
					activated = append(activated, payload.Name)
				}
			}
			if failAt == 0 && (len(activated) != 2 || activated[0] != "z-sql.container" || activated[1] != "a-app.container") {
				t.Fatal("dependency restarted after application activation", activated)
			}
		})
	}
}

func TestQuadletGraphRejectsDependencyCycleBeforeMutation(t *testing.T) {
	transport := newProjectTestTransport(t, "podman")
	runtime, err := NewProjectRuntime(transport, transport.scope)
	if err != nil {
		t.Fatal(err)
	}
	project, err := runtime.Stage(context.Background(), "owned", []ProjectFile{
		{Path: "a.container", Data: []byte("[Unit]\nRequires=b.service\n[Container]\nImage=example:fixture\n")},
		{Path: "b.container", Data: []byte("[Unit]\nAfter=a.service\n[Container]\nImage=example:fixture\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := len(transport.calls)
	if runtime.ApplyQuadletGraph(context.Background(), project, []string{"a.container", "b.container"}) == nil || len(transport.calls) != before {
		t.Fatal("cyclic graph caused a partial publication or activation")
	}
}
