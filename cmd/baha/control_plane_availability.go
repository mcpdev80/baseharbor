package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/health"
)

type controlPlaneAvailability struct {
	PostgresMembers int
	PostgresEtcd    int
	OpenBaoMembers  int
	Satisfied       bool
}

func collectControlPlaneAvailability(ctx context.Context, checks []health.Check) (controlPlaneAvailability, error) {
	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return controlPlaneAvailability{}, err
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return controlPlaneAvailability{}, err
	}
	runtimeProvider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return controlPlaneAvailability{}, err
	}
	running, err := runtimeProvider.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
	if err != nil {
		return controlPlaneAvailability{}, err
	}
	return evaluateControlPlaneAvailability(running, checks), nil
}

func evaluateControlPlaneAvailability(running []string, checks []health.Check) controlPlaneAvailability {
	runningSet := make(map[string]struct{}, len(running))
	for _, service := range running {
		runningSet[strings.TrimSpace(service)] = struct{}{}
	}
	count := func(prefix string, max int) int {
		total := 0
		for ordinal := 1; ordinal <= max; ordinal++ {
			if _, ok := runningSet[fmt.Sprintf("%s-%d", prefix, ordinal)]; ok {
				total++
			}
		}
		return total
	}
	checkOK := func(name string) bool {
		for _, check := range checks {
			if check.Name == name {
				return check.OK
			}
		}
		return false
	}

	report := controlPlaneAvailability{
		PostgresMembers: count("postgres-member", 3),
		PostgresEtcd:    count("postgres-etcd", 3),
		OpenBaoMembers:  count("openbao-member", 3),
	}
	postgresSatisfied := report.PostgresMembers >= 2 && report.PostgresEtcd >= 2 && checkOK("postgres")
	openBaoSatisfied := report.OpenBaoMembers >= 2 && checkOK("openbao")
	report.Satisfied = postgresSatisfied && openBaoSatisfied
	return report
}

func (r controlPlaneAvailability) Detail() string {
	return fmt.Sprintf(
		"requested=ha resolved=postgresql:3,openbao:3 failure-domain=runtime-host host-failure-tolerance=false running=postgresql:%d/3,etcd:%d/3,openbao:%d/3 satisfied=%t",
		r.PostgresMembers,
		r.PostgresEtcd,
		r.OpenBaoMembers,
		r.Satisfied,
	)
}
