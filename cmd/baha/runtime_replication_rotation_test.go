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
	const harness = `import io, json, os, sys, tempfile, types, urllib.request
script = sys.stdin.read()
# Exercise the local-configuration precedence failure without requiring PyYAML
# on the source-test host. The real Spilo image supplies PyYAML.
sys.modules['yaml'] = types.SimpleNamespace(safe_load=json.load, safe_dump=json.dump)
base = ['local all all trust', 'hostssl replication old all md5', 'hostssl all all 0.0.0.0/0 scram-sha-256']
class Response(io.StringIO):
    status = 202
with tempfile.TemporaryDirectory() as directory:
    path = os.path.join(directory, 'postgres.yml')
    script = script.replace("'/home/postgres/postgres.yml'", repr(path))
    def run(rules):
        with open(path, 'w') as target:
            json.dump({'postgresql': {'pg_hba': list(rules)}, 'restapi': {'listen': ':8008'}}, target)
        os.chmod(path, 0o640)
        def request(value, timeout):
            assert value.method == 'POST' and value.full_url.endswith('/reload')
            return Response('{}')
        urllib.request.urlopen = request
        sys.argv = ['probe', 'old', 'new']
        exec(script, {})
        assert os.stat(path).st_mode & 0o777 == 0o640
        with open(path) as source:
            config = json.load(source)
        assert config['restapi'] == {'listen': ':8008'}
        return config['postgresql']['pg_hba']
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
