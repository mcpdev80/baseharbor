package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/health"
)

type controlPlaneAvailability struct {
	HA              bool
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
	return evaluateControlPlaneAvailability(running, checks, files.HA), nil
}

func evaluateControlPlaneAvailability(running []string, checks []health.Check, ha bool) controlPlaneAvailability {
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
		HA:              ha,
		PostgresMembers: count("postgres-member", 3),
		PostgresEtcd:    count("postgres-etcd", 3),
		OpenBaoMembers:  count("openbao-member", 3),
	}
	postgresSatisfied := report.PostgresMembers >= 2 && report.PostgresEtcd >= 2 && checkOK("postgres")
	openBaoSatisfied := report.OpenBaoMembers >= 2 && checkOK("openbao")
	report.Satisfied = postgresSatisfied && openBaoSatisfied
	if !ha {
		report.Satisfied = report.PostgresMembers == 1 && report.PostgresEtcd == 0 && report.OpenBaoMembers == 1 && checkOK("postgres") && checkOK("openbao")
	}
	return report
}

func (r controlPlaneAvailability) Detail() string {
	if !r.HA {
		return fmt.Sprintf("requested=single resolved=postgresql:1,openbao:1 failover=false running=postgresql:%d/1,etcd:%d/0,openbao:%d/1 satisfied=%t", r.PostgresMembers, r.PostgresEtcd, r.OpenBaoMembers, r.Satisfied)
	}
	return fmt.Sprintf(
		"requested=ha resolved=postgresql:3,openbao:3 failure-domain=runtime-host host-failure-tolerance=false running=postgresql:%d/3,etcd:%d/3,openbao:%d/3 satisfied=%t",
		r.PostgresMembers,
		r.PostgresEtcd,
		r.OpenBaoMembers,
		r.Satisfied,
	)
}
