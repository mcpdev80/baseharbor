package main

import (
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestCoreStopContainerNameUsesCanonicalQuadletIdentity(t *testing.T) {
	project := "bh-local-shared"
	service := "postgres-admin"
	ordinal := project + "-" + service + "-1"
	quadlet := project + "-" + service
	pinned := "custom-postgres-admin"
	cases := []struct {
		name    string
		kind    bhruntime.ProviderKind
		actual  bhruntime.RuntimeContainer
		planned []string
		want    bool
	}{
		{name: "podman canonical quadlet", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: quadlet}, planned: []string{ordinal}, want: true},
		{name: "podman canonical ordinal", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: ordinal}, planned: []string{ordinal}, want: true},
		{name: "docker canonical ordinal", kind: bhruntime.ProviderDocker, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: ordinal}, planned: []string{ordinal}, want: true},
		{name: "docker cannot use podman alias", kind: bhruntime.ProviderDocker, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: quadlet}, planned: []string{ordinal}},
		{name: "podman pinned exact", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: pinned}, planned: []string{pinned}, want: true},
		{name: "podman native cannot bypass pinned", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: quadlet}, planned: []string{pinned}},
		{name: "foreign project rejected", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: "foreign", Service: service, Name: quadlet}, planned: []string{ordinal}},
		{name: "foreign service rejected", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: project, Service: "openbao", Name: quadlet}, planned: []string{ordinal}},
		{name: "wrong native name rejected", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: "foreign"}, planned: []string{ordinal}},
		{name: "scaled service cannot use singleton quadlet", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: quadlet}, planned: []string{ordinal, project + "-" + service + "-2"}},
		{name: "missing planned service rejected", kind: bhruntime.ProviderPodman, actual: bhruntime.RuntimeContainer{Project: project, Service: service, Name: quadlet}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := coreStopContainerNameIsPlanned(tc.kind, tc.actual, project, tc.planned); got != tc.want {
				t.Fatalf("core stop native identity admitted=%v, want=%v", got, tc.want)
			}
		})
	}
}
