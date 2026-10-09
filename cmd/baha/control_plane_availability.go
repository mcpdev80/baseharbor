package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
)

type controlPlaneAvailability struct {
	HA                          bool
	PostgresMembers             int
	PostgresEtcd                int
	OpenBaoMembers              int
	Helpers                     int
	ActualHA                    bool
	Satisfied                   bool
	IdentityMembers             int
	IdentitySQLMembers          int
	IdentityEtcd                int
	PostgresReplicationVerified bool
	IdentityReplicationVerified bool
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
	report := evaluateControlPlaneAvailability(running, checks, files.HA)
	dataDir, err := targetDataRoot(target)
	if err != nil {
		return controlPlaneAvailability{}, err
	}
	identity, err := identityprovider.ExistingCoreRuntimeFiles(dataDir, target.Name)
	if err != nil {
		return controlPlaneAvailability{}, err
	}
	identityRunning, err := runtimeProvider.RunningServicesProject(ctx, identity.Project, identity.Compose, identity.Env)
	if err != nil {
		return controlPlaneAvailability{}, err
	}
	report.observeIdentity(identityRunning)
	if files.HA {
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		report.PostgresReplicationVerified = observePatroniReplication(probeCtx, runtimeProvider, files.Project, files.Compose, files.Env, "postgres-member")
		report.IdentityReplicationVerified = observePatroniReplication(probeCtx, runtimeProvider, identity.Project, identity.Compose, identity.Env, "keycloak-db-member")
	}
	return report, nil
}

func (r *controlPlaneAvailability) observeIdentity(running []string) {
	member := func(name, prefix string) bool {
		n, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
		return strings.HasPrefix(name, prefix) && err == nil && n > 0
	}
	for _, name := range running {
		switch {
		case member(name, "keycloak-db-member-") || name == "keycloak-db" && !r.HA:
			// In HA keycloak-db is the stable SQL proxy, not a data member.
			if name != "keycloak-db" || !r.HA {
				r.IdentitySQLMembers++
			}
		case member(name, "keycloak-db-etcd-"):
			r.IdentityEtcd++
		case member(name, "keycloak-"):
			r.IdentityMembers++
		case strings.HasPrefix(name, "keycloak-"):
			r.Helpers++
		}
	}
	identitySatisfied := r.IdentityMembers == 1 && r.IdentitySQLMembers == 1 && r.IdentityEtcd == 0
	if r.HA {
		identitySatisfied = r.IdentityMembers >= 2 && r.IdentitySQLMembers >= 2 && r.IdentityEtcd >= 2
	}
	r.Satisfied = r.Satisfied && identitySatisfied
	r.ActualHA = r.ActualHA && r.IdentityMembers >= 2 && r.IdentitySQLMembers >= 2 && r.IdentityEtcd >= 2
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
	for _, helper := range []string{"postgres", "postgres-admin", "postgres-init", "openbao", "openbao-admin"} {
		if _, ok := runningSet[helper]; ok {
			report.Helpers++
		}
	}
	postgresSatisfied := report.PostgresMembers >= 2 && report.PostgresEtcd >= 2 && checkOK("postgres")
	openBaoSatisfied := report.OpenBaoMembers >= 2 && checkOK("openbao")
	report.Satisfied = postgresSatisfied && openBaoSatisfied
	report.ActualHA = report.Satisfied
	if !ha {
		report.Satisfied = report.PostgresMembers == 1 && report.PostgresEtcd == 0 && report.OpenBaoMembers == 1 && checkOK("postgres") && checkOK("openbao")
	}
	return report
}

func (r controlPlaneAvailability) Detail() string {
	replicas := r.PostgresMembers - 1
	if replicas < 0 {
		replicas = 0
	}
	active, proof, actualReplicas := "false", "not-applicable", "0"
	identityProof, identityReplicas := "not-applicable", "0"
	if r.HA {
		proof, identityProof, actualReplicas, identityReplicas = "unverified", "unverified", "unknown", "unknown"
		if r.PostgresReplicationVerified {
			proof, actualReplicas = "native-primary-two-streaming-replicas", "2"
		}
		if r.IdentityReplicationVerified {
			identityProof, identityReplicas = "native-primary-two-streaming-replicas", "2"
		}
		if r.ActualHA {
			active = "unknown"
			if r.PostgresReplicationVerified && r.IdentityReplicationVerified {
				active = "true"
			}
		}
	}
	topology := fmt.Sprintf(" ha-requested=%t ha-active=%s data-members=postgresql:%d,openbao-sql:shared configured-replicas=postgresql:%d,openbao-sql:0 replicas=postgresql:%s auxiliary-services=%d openbao-service-members=%d replication-proof=%s process-failover-capable=%t failover-proof=not-collected", r.HA, active, r.PostgresMembers, replicas, actualReplicas, r.Helpers, r.OpenBaoMembers, proof, r.ActualHA)
	topology += fmt.Sprintf(" identity-service-members=%d identity-sql-data-members=%d identity-etcd-members=%d", r.IdentityMembers, r.IdentitySQLMembers, r.IdentityEtcd)
	topology += " identity-sql-replicas=" + identityReplicas + " identity-sql-replication-proof=" + identityProof
	if !r.HA {
		return fmt.Sprintf("requested=single resolved=postgresql:1,openbao:1 failover=false running=postgresql:%d/1,etcd:%d/0,openbao:%d/1 satisfied=%t", r.PostgresMembers, r.PostgresEtcd, r.OpenBaoMembers, r.Satisfied) + topology
	}
	return fmt.Sprintf(
		"requested=ha resolved=postgresql:3,openbao:3 failure-domain=runtime-host host-failure-tolerance=false running=postgresql:%d/3,etcd:%d/3,openbao:%d/3 satisfied=%t",
		r.PostgresMembers,
		r.PostgresEtcd,
		r.OpenBaoMembers,
		r.Satisfied,
	) + topology
}
