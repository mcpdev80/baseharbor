package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type retirementRuntime struct {
	before, after          []ProjectResource
	inspectErr, downErr    error
	inspections, mutations int
}

func (r *retirementRuntime) InspectProjectResources(context.Context, string, []ProjectResource) ([]ProjectResource, error) {
	r.inspections++
	if r.inspectErr != nil {
		return nil, r.inspectErr
	}
	if r.inspections == 1 {
		return r.before, nil
	}
	return r.after, nil
}

func (r *retirementRuntime) DownProject(context.Context, string, string, string) error {
	r.mutations++
	return r.downErr
}

func TestRetireOwnedRuntimePreservesDataAndBusyNetwork(t *testing.T) {
	container := ProjectResource{Kind: "container", Name: "identity"}
	volume := ProjectResource{Kind: "volume", Name: "identity-data"}
	network := ProjectResource{Kind: "network", Name: "shared"}
	r := &retirementRuntime{before: []ProjectResource{container, volume, network}, after: []ProjectResource{volume, network}}
	result, err := RetireOwnedRuntime(context.Background(), r, Files{Project: "owned", Compose: "owned.yaml", Env: "owned.env"}, r.before)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Removed, []ProjectResource{container}) || len(result.Retained) != 2 || r.mutations != 1 {
		t.Fatalf("unexpected retirement: %+v", result)
	}
	for _, item := range result.Retained {
		if item.Reason == "" {
			t.Fatal("retained resource lacks explanation")
		}
	}
}

func TestRetireOwnedRuntimeRejectsForeignInventoryBeforeMutation(t *testing.T) {
	for _, inspectErr := range []error{ErrResourceOwnership, errors.New("partial ownership state")} {
		r := &retirementRuntime{inspectErr: inspectErr}
		_, err := RetireOwnedRuntime(context.Background(), r, Files{Project: "owned", Compose: "owned.yaml", Env: "owned.env"}, []ProjectResource{{Kind: "container", Name: "foreign"}})
		if !errors.Is(err, inspectErr) || r.mutations != 0 {
			t.Fatalf("unsafe retirement: %v, mutations=%d", err, r.mutations)
		}
	}
}

func TestRetireOwnedRuntimeDoesNotReportSuccessAfterIncompleteRemoval(t *testing.T) {
	container := ProjectResource{Kind: "container", Name: "identity"}
	volume := ProjectResource{Kind: "volume", Name: "identity-data"}
	for _, r := range []*retirementRuntime{
		{before: []ProjectResource{container, volume}, after: []ProjectResource{container, volume}},
		{before: []ProjectResource{container, volume}},
		{before: []ProjectResource{container, volume}, downErr: errors.New("interrupted apply")},
	} {
		_, err := RetireOwnedRuntime(context.Background(), r, Files{Project: "owned", Compose: "owned.yaml", Env: "owned.env"}, r.before)
		if err == nil {
			t.Fatal("incomplete retirement reported success")
		}
	}
}

func TestRetireOwnedRuntimeRepeatedRemovalKeepsDataEvidence(t *testing.T) {
	volume := ProjectResource{Kind: "volume", Name: "identity-data"}
	r := &retirementRuntime{before: []ProjectResource{volume}, after: []ProjectResource{volume}}
	result, err := RetireOwnedRuntime(context.Background(), r, Files{Project: "owned", Compose: "owned.yaml", Env: "owned.env"}, []ProjectResource{{Kind: "container", Name: "identity"}, volume})
	if err != nil || len(result.Removed) != 0 || len(result.Retained) != 1 {
		t.Fatalf("repeat retirement: %+v, %v", result, err)
	}
}

func TestRetireOwnedRuntimeRejectsUnplannedObservation(t *testing.T) {
	r := &retirementRuntime{before: []ProjectResource{{Kind: "container", Name: "foreign"}}}
	_, err := RetireOwnedRuntime(context.Background(), r, Files{Project: "owned", Compose: "owned.yaml", Env: "owned.env"}, []ProjectResource{{Kind: "container", Name: "owned"}})
	if err == nil || r.mutations != 0 {
		t.Fatal("unplanned observation authorized mutation")
	}
}

func TestInterruptedRetirementReturnsOnlyVerifiedActualDisposition(t *testing.T) {
	container := ProjectResource{Kind: "container", Name: "identity"}
	volume := ProjectResource{Kind: "volume", Name: "identity-data"}
	interrupted := errors.New("interrupted")
	for _, after := range [][]ProjectResource{{volume}, {container}} {
		r := &retirementRuntime{before: []ProjectResource{container, volume}, after: after, downErr: interrupted}
		result, err := RetireOwnedRuntime(context.Background(), r, Files{Project: "owned", Compose: "owned.yaml", Env: "owned.env"}, r.before)
		if !errors.Is(err, interrupted) || r.inspections != 2 {
			t.Fatalf("interruption concealed or actual disposition not inspected: %+v %v", result, err)
		}
		retainedVolume := false
		for _, retained := range result.Retained {
			retainedVolume = retainedVolume || retained.Resource == volume
		}
		if retainedVolume != reflect.DeepEqual(after, []ProjectResource{volume}) {
			t.Fatalf("initial inventory was incorrectly presented as final preservation: %+v", result)
		}
	}
}
