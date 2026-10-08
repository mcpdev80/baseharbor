package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreupdate"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type patroniInspectionRuntime struct {
	bhruntime.RuntimeProvider
	role map[string]string
}

func (r patroniInspectionRuntime) ExecProject(_ context.Context, _, _, _, service string, argv ...string) (string, error) {
	if len(argv) >= 3 && argv[2] == "import urllib.request,sys; sys.stdout.write(urllib.request.urlopen('http://127.0.0.1:8008/cluster',timeout=3).read().decode('utf-8'))" {
		return `{"members":[{"name":"postgres-member-1","role":"leader","state":"running"},{"name":"postgres-member-2","role":"replica","state":"streaming","lag":0},{"name":"postgres-member-3","role":"replica","state":"streaming","lag":0}]}`, nil
	}
	if len(argv) < 3 {
		return "", errors.New("invalid probe")
	}
	path := ""
	if argv[2] == "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8008/primary',timeout=3).close()" {
		path = "primary"
	} else if argv[2] == "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8008/replica',timeout=3).close()" {
		path = "replica"
	}
	if r.role[service] != path || path == "" {
		return "", errors.New("not healthy for role")
	}
	return "", nil
}
func TestPatroniRuntimeInspectionRejectsSplitBrainAndMissingReplica(t *testing.T) {
	files := bhruntime.Files{HA: true, Project: "owned", Compose: "owned.yaml", Env: "owned.env"}
	roles := map[string]string{"postgres-member-1": "primary", "postgres-member-2": "replica", "postgres-member-3": "replica"}
	rt := patroniInspectionRuntime{role: roles}
	members, err := inspectPatroniMembers(context.Background(), rt, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := coreupdate.VerifyPatroniQuorum(context.Background(), members, 0); err != nil {
		t.Fatal(err)
	}
	roles["postgres-member-3"] = "primary"
	if members, err := inspectPatroniMembers(context.Background(), rt, files); err != nil {
		t.Fatal(err)
	} else if _, _, err := coreupdate.VerifyPatroniQuorum(context.Background(), members, 0); err == nil {
		t.Fatal("two active primaries accepted")
	}
	delete(roles, "postgres-member-3")
	if _, err := inspectPatroniMembers(context.Background(), rt, files); err == nil {
		t.Fatal("missing replica accepted")
	}
	files.HA = false
	if _, err := inspectPatroniMembers(context.Background(), rt, files); err == nil {
		t.Fatal("single-node topology accepted as Patroni HA")
	}
}
