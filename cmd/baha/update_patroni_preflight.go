package main

import (
    "context"
    "errors"
    "fmt"

    "github.com/mcpdev80/baseharbor/internal/coreupdate"
    bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// inspectPatroniMembers uses the running members' local Patroni REST API.
// A service name or a functioning proxy is not evidence of a healthy quorum.
func inspectPatroniMembers(ctx context.Context, rt bhruntime.RuntimeProvider, files bhruntime.Files) ([]coreupdate.PatroniMemberState, error) {
    if !files.HA {
        return nil, errors.New("Patroni membership requires an HA Core")
    }
    if rt == nil || files.Project == "" || files.Compose == "" || files.Env == "" {
        return nil, errors.New("Patroni runtime and managed project identity required")
    }
    const probe = "import json,urllib.request,sys\ntry:\n d=json.load(urllib.request.urlopen('http://127.0.0.1:8008/patroni', timeout=3))\n role=d.get('role',''); state=d.get('state',''); lag=d.get('xlog',{}).get('replayed_location'); print(json.dumps({'role':role,'state':state}))\nexcept Exception:\n sys.exit(1)"
    members := make([]coreupdate.PatroniMemberState, 0, 3)
    for _, name := range []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"} {
        if err := ctx.Err(); err != nil { return nil, err }
        // Patroni's primary and replica endpoints are health-role assertions;
        // probe each independently and never infer a role from an error body.
        const primary = "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8008/primary',timeout=3).close()"
        const replica = "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8008/replica',timeout=3).close()"
        _, pErr := rt.ExecProject(ctx, files.Project, files.Compose, files.Env, name, "python3", "-c", primary)
        _, rErr := rt.ExecProject(ctx, files.Project, files.Compose, files.Env, name, "python3", "-c", replica)
        if (pErr == nil) == (rErr == nil) {
            return nil, fmt.Errorf("Patroni member %s has ambiguous role/health", name)
        }
        members = append(members, coreupdate.PatroniMemberState{
            Name: name, Primary: pErr == nil, Replica: rErr == nil, Healthy: true,
        })
    }
    _ = probe // preserved as schema documentation for a future lag-aware probe
    return members, nil
}
