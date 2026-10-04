package main

import (
	"context"
	"fmt"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Spilo only generates the original replication HBA rule at bootstrap. Creating
// a replacement SQL role does not authorize it for physical replication. Keep
// both identities admitted in Patroni's DCS before recreating any member.
const controlPlaneReplicationHBAPatch = `import json, os, subprocess, sys, urllib.request
old, new = sys.argv[1:]
url = 'http://127.0.0.1:8008/config'
config = json.load(urllib.request.urlopen(url, timeout=5))
rules = config.get('postgresql', {}).get('pg_hba')
if rules is None:
    path = subprocess.check_output(['psql', '-U', os.environ['PGUSER_SUPERUSER'], '-d', 'postgres', '-Atqc', 'SHOW hba_file'], text=True).strip()
    with open(path) as source:
        rules = source.read().splitlines()
replacement = []
for rule in rules:
    fields = rule.split()
    if len(fields) >= 5 and fields[0] == 'hostssl' and fields[1] == 'replication' and fields[2] == old:
        fields[2] = new
        replacement.append(' '.join(fields))
if not replacement:
    raise RuntimeError('no TLS replication HBA rule for previous identity')
for rule in replacement:
    if rule not in rules:
        rules.insert(0, rule)
request = urllib.request.Request(url, data=json.dumps({'postgresql': {'pg_hba': rules}}).encode(), headers={'Content-Type': 'application/json'}, method='PATCH')
with urllib.request.urlopen(request, timeout=5) as response:
    if response.status != 200:
        raise RuntimeError('Patroni rejected replication HBA overlap')
`

func prepareControlPlaneReplicationOverlap(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, previous string, next bhruntime.ControlPlaneCredentials) error {
	primary, err := controlPlanePostgresPrimary(ctx, runtime, files)
	if err != nil {
		return err
	}
	if _, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, primary, "python3", "-c", controlPlaneReplicationHBAPatch, previous, next.PostgresReplicationUser); err != nil {
		return fmt.Errorf("prepare Patroni replication HBA overlap: %w", err)
	}
	// Prove actual physical-replication authentication on every member. A normal
	// SELECT 1 uses database HBA rules and cannot detect this failure.
	const probe = "IFS= read -r PGPASSWORD\nexport PGPASSWORD\nexport PGSSLMODE=verify-full PGSSLROOTCERT=/run/baseharbor/postgres-ca/ca.pem PGCONNECT_TIMEOUT=2\nexec psql \"host=$1 user=$2 replication=true\" -v ON_ERROR_STOP=1 -c IDENTIFY_SYSTEM"
	for _, member := range []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"} {
		deadline := time.Now().Add(30 * time.Second)
		for {
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err = runtime.ExecProjectInput(probeCtx, files.Project, files.Compose, files.Env, []byte(next.PostgresReplicationPass+"\n"), "postgres-admin", "sh", "-ec", probe, "--", member, next.PostgresReplicationUser)
			cancel()
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("verify replacement physical replication credential on %s: %w", member, err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}
