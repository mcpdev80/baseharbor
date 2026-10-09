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
	return inspectPatroniReplication(ctx, rt, project, compose, env, prefix) == nil
}

// Return fixed diagnostic classes, never native output or credentials.
func inspectPatroniReplication(ctx context.Context, rt bhruntime.RuntimeProvider, project, compose, env, prefix string) error {
	if rt == nil || project == "" || compose == "" || env == "" {
		return fmt.Errorf("replication context unavailable")
	}
	const clusterProbe = "import urllib.request,sys; sys.stdout.write(urllib.request.urlopen('http://127.0.0.1:8008/cluster',timeout=3).read().decode('utf-8'))"
	output, err := rt.ExecProject(ctx, project, compose, env, prefix+"-1", "python3", "-c", clusterProbe)
	if err != nil {
		return fmt.Errorf("cluster inventory unavailable")
	}
	var cluster struct {
		Members []struct {
			Name, Role, State string
			Lag               *int64
		}
	}
	if json.Unmarshal([]byte(output), &cluster) != nil || len(cluster.Members) != 3 {
		return fmt.Errorf("cluster inventory incomplete")
	}
	expected := map[string]bool{}
	for i := 1; i <= 3; i++ {
		expected[fmt.Sprintf("%s-%d", prefix, i)] = true
	}
	leaders, replicas := 0, 0
	for _, member := range cluster.Members {
		if !expected[member.Name] {
			return fmt.Errorf("cluster member ownership differs")
		}
		delete(expected, member.Name)
		endpoint := ""
		switch {
		case member.Role == "leader" && member.State == "running":
			leaders++
			endpoint = "primary"
		case (member.Role == "replica" || member.Role == "sync_standby" || member.Role == "quorum_standby") && (member.State == "running" || member.State == "streaming") && member.Lag != nil && *member.Lag >= 0:
			replicas++
			endpoint = "replica"
		default:
			return fmt.Errorf("cluster member role, state or lag not ready")
		}
		probe := "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8008/" + endpoint + "',timeout=3).close()"
		if _, err := rt.ExecProject(ctx, project, compose, env, member.Name, "python3", "-c", probe); err != nil {
			return fmt.Errorf("native member health role not ready")
		}
	}
	if leaders != 1 || replicas != 2 || len(expected) != 0 {
		return fmt.Errorf("cluster primary/replica membership differs")
	}
	return nil
}
