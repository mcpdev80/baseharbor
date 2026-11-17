package main

import (
	"reflect"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestDestroySelectsOnlyExclusivelyOwnedApplicationDataVolumes(t *testing.T) {
	volumes := []bhruntime.RepositoryVolume{
		{Name: "app-data", Project: "demo-workload"},
		{Name: "shared-data", Project: "demo-workload", Shared: true, Reason: "shared consumer"},
		{Name: "foreign-data", Project: "other-workload"},
		{Name: "unknown-data", Reason: "mounted volume without verified ownership"},
		{Name: "", Project: "demo-workload"},
	}
	want := []bhruntime.ProjectResource{{Kind: "volume", Name: "app-data"}}
	if got := exclusivelyOwnedRepositoryVolumes(volumes, "demo-workload"); !reflect.DeepEqual(got, want) {
		t.Fatalf("destroy selected unsafe or missed owned data volumes: got=%+v want=%+v", got, want)
	}
}
