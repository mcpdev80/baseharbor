package main

import (
	"context"
	"encoding/json"
	"fmt"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Observe only native health-role assertions and the owned cluster inventory.
// This does not perform a switchover or claim an observed failover.
func observePatroniReplication(ctx context.Context, rt bhruntime.RuntimeProvider, project, compose, env, prefix string) bool {
	if rt == nil || project == "" || compose == "" || env == "" {
		return false
	}
	const clusterProbe = "import urllib.request,sys; sys.stdout.write(urllib.request.urlopen('http://127.0.0.1:8008/cluster',timeout=3).read().decode('utf-8'))"
	output, err := rt.ExecProject(ctx, project, compose, env, prefix+"-1", "python3", "-c", clusterProbe)
	if err != nil {
		return false
	}
	var cluster struct {
		Members []struct {
			Name, Role, State string
			Lag               *int64
		}
	}
	if json.Unmarshal([]byte(output), &cluster) != nil || len(cluster.Members) != 3 {
		return false
	}
	expected := map[string]bool{}
	for i := 1; i <= 3; i++ {
		expected[fmt.Sprintf("%s-%d", prefix, i)] = true
	}
	leaders, replicas := 0, 0
	for _, member := range cluster.Members {
		if !expected[member.Name] {
			return false
		}
		delete(expected, member.Name)
		endpoint := ""
		switch {
		case member.Role == "leader" && member.State == "running":
			leaders++
			endpoint = "primary"
		case member.Role == "replica" && (member.State == "running" || member.State == "streaming") && member.Lag != nil && *member.Lag >= 0:
			replicas++
			endpoint = "replica"
		default:
			return false
		}
		probe := "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8008/" + endpoint + "',timeout=3).close()"
		if _, err := rt.ExecProject(ctx, project, compose, env, member.Name, "python3", "-c", probe); err != nil {
			return false
		}
	}
	return leaders == 1 && replicas == 2 && len(expected) == 0
}
