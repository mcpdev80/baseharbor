package main

import (
	"os/exec"
	"testing"
)

func TestReplicationHBAOverlapPreservesRulesAndRejectsMissingIdentity(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to exercise the Patroni projection")
	}
	const harness = `import io, json, sys, urllib.request
script = sys.stdin.read()
base = ['local all all trust', 'hostssl replication old all md5', 'hostssl all all 0.0.0.0/0 scram-sha-256']
class Response(io.StringIO):
    status = 200
def run(rules):
    patches = []
    def request(value, timeout):
        if isinstance(value, str):
            return Response(json.dumps({'postgresql': {'pg_hba': list(rules)}}))
        patches.append(json.loads(value.data))
        return Response('{}')
    urllib.request.urlopen = request
    sys.argv = ['probe', 'old', 'new']
    exec(script, {})
    return patches[0]['postgresql']['pg_hba']
rules = run(base)
assert all(rule in rules for rule in base), rules
assert rules[0] == 'hostssl replication new all md5', rules
assert run(rules) == rules, 'resume must be idempotent'
try:
    run(['hostssl all all 0.0.0.0/0 scram-sha-256'])
except RuntimeError:
    pass
else:
    raise AssertionError('missing old replication rule must fail closed')
`
	cmd := exec.Command(python, "-c", harness)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer stdin.Close()
		_, _ = stdin.Write([]byte(controlPlaneReplicationHBAPatch))
	}()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("replication HBA overlap: %v\n%s", err, out)
	}
}
