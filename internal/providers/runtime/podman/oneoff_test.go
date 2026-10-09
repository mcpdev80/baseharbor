package podman

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeOneOffRecoveryPreservesSecurityAndStreamContract(t *testing.T) {
	r, err := parseNativeOneOff([]string{"--rm", "--no-deps", "--user", "1001:1001", "-v", "/owned/snapshot:/snapshot:ro", "--entrypoint", "/usr/local/bin/etcdutl", "recovery", "snapshot", "restore", "/snapshot/etcd.snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	model := quadletComposeProject{
		Services: map[string]quadletComposeService{"recovery": {
			Image: "etcd@sha256:" + strings.Repeat("a", 64), ReadOnly: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"},
			Volumes: []string{"./pki:/run/etcd:ro"}, Tmpfs: []string{"/tmp:rw,noexec,nosuid,nodev"}, Environment: composeEnv{"TEST_SECRET": "secret-not-in-argv"}, Profiles: []string{"recovery"},
		}}, Networks: map[string]quadletComposeResource{"default": {}},
	}
	args, env, resources, err := nativeOneOffArgs("core", "/owned/compose.yaml", model, r)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, wanted := range []string{"run --rm -i", "--read-only", "--cap-drop ALL", "--security-opt no-new-privileges:true", "--user 1001:1001", "--network core_default", "--volume /owned/pki:/run/etcd:ro", "--volume /owned/snapshot:/snapshot:ro", "snapshot restore /snapshot/etcd.snapshot"} {
		if !strings.Contains(joined, wanted) {
			t.Fatalf("missing native recovery contract %q in %s", wanted, joined)
		}
	}
	if strings.Contains(joined, env["TEST_SECRET"]) || env["TEST_SECRET"] != "secret-not-in-argv" {
		t.Fatal("secret argv leak or environment lost")
	}
	if len(resources) != 1 || resources[0].Name != "core_default" {
		t.Fatal("managed network ownership verification missing")
	}
	for i, arg := range args {
		if arg == "--entrypoint" {
			var entry []string
			if err := json.Unmarshal([]byte(args[i+1]), &entry); err != nil || len(entry) != 1 || entry[0] != "/usr/local/bin/etcdutl" {
				t.Fatal("native entrypoint lost")
			}
		}
	}
}

func TestNativeOneOffRejectsUnboundedOptionsAndUndeclaredServices(t *testing.T) {
	for _, args := range [][]string{
		{"--rm", "recovery"}, {"--no-deps", "recovery"}, {"--rm", "--no-deps", "--privileged", "recovery"},
		{"--rm", "--no-deps", "--volume"}, {"--rm", "--no-deps"},
	} {
		if _, err := parseNativeOneOff(args); err == nil {
			t.Fatalf("unsafe one-off accepted: %v", args)
		}
	}
	if _, _, _, err := nativeOneOffArgs("core", "/owned/compose.yaml", quadletComposeProject{}, nativeOneOffRequest{service: "foreign"}); err == nil {
		t.Fatal("undeclared helper accepted")
	}
}

func TestNativeOneOffOverridesMountWithoutPublishingPortsOrStartingDependencies(t *testing.T) {
	model := quadletComposeProject{Services: map[string]quadletComposeService{"tool": {
		Image: "tool:1", Volumes: []string{"original:/data", "./pki:/pki:ro"}, Ports: []string{"5432:5432"},
		DependsOn: quadletDependencies{Names: []string{"database"}},
	}}, Volumes: map[string]quadletComposeResource{"original": {}}, Networks: map[string]quadletComposeResource{"default": {}}}
	args, _, _, err := nativeOneOffArgs("core", filepath.Join("/owned", "compose.yaml"), model, nativeOneOffRequest{service: "tool", volumes: []string{"/owned/restored:/data"}})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "core_original") || strings.Contains(joined, "5432") || strings.Contains(joined, "database") || !strings.Contains(joined, "/owned/restored:/data") {
		t.Fatalf("one-off crossed service boundary: %s", joined)
	}
}
