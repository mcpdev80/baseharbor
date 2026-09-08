package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkloadManifestRoundTrip(t *testing.T) {
	m := New("mailflow", "prod", true, true, false)
	m = WithWorkload(m, "infrastructure/docker-compose.yml", "api", "worker", "web")
	parsed, err := ParseYAML(m.YAML())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Workload.Compose != "infrastructure/docker-compose.yml" {
		t.Fatalf("unexpected compose path %q", parsed.Workload.Compose)
	}
	if strings.Join(parsed.Workload.Services, ",") != "api,web,worker" {
		t.Fatalf("unexpected workload services %#v", parsed.Workload.Services)
	}
}

func TestWorkloadComposePathCannotEscapeRepository(t *testing.T) {
	m := New("demo", "dev", true, false, false)
	m = WithWorkload(m, "../compose.yaml")
	if err := m.Validate(); err == nil {
		t.Fatal("expected escaping workload compose path to be rejected")
	}
}

func TestResolveWorkloadComposeDetectsUnambiguousConvention(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services:\n  api:\n    image: alpine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, found, err := ResolveWorkloadCompose(root, New("demo", "dev", true, false, false))
	if err != nil || !found {
		t.Fatalf("resolve workload compose: found=%v err=%v", found, err)
	}
	if resolved != path {
		t.Fatalf("got %q want %q", resolved, path)
	}
}

func TestResolveWorkloadComposeFailsClosedWhenAmbiguous(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"compose.yaml", "docker-compose.yml"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("services:\n  api:\n    image: alpine\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := ResolveWorkloadCompose(root, New("demo", "dev", true, false, false))
	if !errors.Is(err, ErrWorkloadComposeAmbiguous) {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestMaterializeWorkloadUsesContainerDNSAndPreservesHostContract(t *testing.T) {
	root := t.TempDir()
	composePath := filepath.Join(root, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("services:\n  api:\n    image: alpine\n    networks:\n      - app-internal\nnetworks:\n  app-internal:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	m := New("demo", "dev", true, true, false)
	m = WithWorkload(m, "docker-compose.yml", "api")
	files, err := EnsureRuntime(store, m)
	if err != nil {
		t.Fatal(err)
	}
	hostEnv, err := os.ReadFile(files.ApplicationEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hostEnv), "@127.0.0.1:") {
		t.Fatalf("host application contract is not loopback-only:\n%s", hostEnv)
	}
	workload, found, err := MaterializeWorkload(root, m, files)
	if err != nil || !found {
		t.Fatalf("materialize workload: found=%v err=%v", found, err)
	}
	override, err := os.ReadFile(workload.Override)
	if err != nil {
		t.Fatal(err)
	}
	text := string(override)
	for _, want := range []string{"@postgres:5432/", "@valkey:6379/0", "baseharbor-backend", ApplicationBackendNetworkName(m)} {
		if !strings.Contains(text, want) {
			t.Fatalf("override missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "127.0.0.1") {
		t.Fatalf("container override leaked host loopback endpoint:\n%s", text)
	}
	if info, err := os.Stat(workload.Override); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("workload override must be owner-only: info=%v err=%v", info, err)
	}
}
