package main

import (
	"context"
	"encoding/json"
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
	members := make([]coreupdate.PatroniMemberState, 0, 3)
	for _, name := range []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
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
	// Validate replication lag from Patroni's cluster inventory; the local
	// /replica health endpoint alone does not establish a bounded WAL lag.
	const clusterScript = "import urllib.request,sys; sys.stdout.write(urllib.request.urlopen('http://127.0.0.1:8008/cluster',timeout=3).read().decode('utf-8'))"
	output, err := rt.ExecProject(ctx, files.Project, files.Compose, files.Env, "postgres-member-1", "python3", "-c", clusterScript)
	if err != nil { return nil, fmt.Errorf("inspect Patroni cluster replication inventory: %w", err) }
	var cluster struct { Members []struct {
		Name string `json:"name"`
		Role string `json:"role"`
		State string `json:"state"`
		Lag *int64 `json:"lag"`
	} `json:"members"` }
	if err := json.Unmarshal([]byte(output), &cluster); err != nil {
		return nil, fmt.Errorf("invalid Patroni cluster inventory: %w", err)
	}
	if len(cluster.Members) != len(members) { return nil, errors.New("Patroni cluster inventory member count mismatch") }
	byName := make(map[string]int, len(members))
	for i, m := range members { byName[m.Name] = i }
	seen := make(map[string]bool, len(members))
	for _, m := range cluster.Members {
		i, ok := byName[m.Name]
		if !ok || seen[m.Name] { return nil, errors.New("Patroni cluster inventory contains foreign or duplicate member") }
		seen[m.Name] = true
		if members[i].Primary {
			if m.Role != "leader" || m.State != "running" { return nil, fmt.Errorf("Patroni member %s leader identity mismatch", m.Name) }
		} else {
			if m.Role != "replica" || (m.State != "running" && m.State != "streaming") || m.Lag == nil || *m.Lag < 0 {
				return nil, fmt.Errorf("Patroni replica %s has unverifiable replay lag or state", m.Name)
			}
			members[i].ReplayLag = *m.Lag
		}
	}
	return members, nil
}
