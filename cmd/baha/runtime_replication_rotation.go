package main

import (
	"context"
	"fmt"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

// Spilo's local pg_hba overrides Patroni DCS configuration. Reload every
// running member with overlapping identities before the first recreate.
const controlPlaneReplicationHBAPatch = `import os, stat, sys, urllib.request, yaml
old, new = sys.argv[1:]
path = os.path.realpath('/home/postgres/postgres.yml')
with open(path) as source:
    config = yaml.safe_load(source)
rules = config['postgresql']['pg_hba']
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
metadata = os.stat(path)
temporary = path + '.baseharbor.tmp'
try:
    with open(temporary, 'w') as target:
        yaml.safe_dump(config, target)
    os.chmod(temporary, stat.S_IMODE(metadata.st_mode))
    os.chown(temporary, metadata.st_uid, metadata.st_gid)
    os.replace(temporary, path)
finally:
    if os.path.exists(temporary):
        os.unlink(temporary)
request = urllib.request.Request('http://127.0.0.1:8008/reload', data=b'{}', headers={'Content-Type': 'application/json'}, method='POST')
with urllib.request.urlopen(request, timeout=5) as response:
    if response.status != 202:
        raise RuntimeError('Patroni rejected local replication HBA reload')
`

func prepareControlPlaneReplicationOverlap(ctx context.Context, runtime bhruntime.RuntimeProvider, files bhruntime.Files, previous string, next bhruntime.ControlPlaneCredentials) error {
	var err error
	for _, member := range []string{"postgres-member-1", "postgres-member-2", "postgres-member-3"} {
		if _, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, member, "python3", "-c", controlPlaneReplicationHBAPatch, previous, next.PostgresReplicationUser); err != nil {
			return fmt.Errorf("prepare Patroni local replication HBA overlap on %s: %w", member, err)
		}
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
