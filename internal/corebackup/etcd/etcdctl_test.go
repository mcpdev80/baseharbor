package etcd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestExternalClientFailsClosedWithoutAttestation(t *testing.T) {
	root := t.TempDir()
	ctl := filepath.Join(root, "etcdctl")
	utl := filepath.Join(root, "etcdutl")
	for _, p := range []string{ctl, utl} {
		if err := os.WriteFile(p, []byte("no execution"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	source := EtcdctlSource{
		Etcdctl: ctl, Etcdutl: utl,
		Endpoints:  []string{"https://etcd-1.internal:2379"},
		Identity:   Identity{Core: "core-123", Target: "target-123", Cluster: "cluster-123"},
		TLS:        TLSFiles{CA: filepath.Join(root, "missing-ca"), Cert: filepath.Join(root, "missing-cert"), Key: filepath.Join(root, "missing-key")},
		ScratchDir: root,
	}
	if _, err := source.Snapshot(context.Background(), os.Stdout); err == nil {
		t.Fatal("missing TLS credential accepted")
	}
	if err := os.Chmod(ctl, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Snapshot(context.Background(), os.Stdout); err == nil {
		t.Fatal("untrusted helper accepted")
	}
}
func TestExternalClientRejectsInsecureEndpoints(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "etcdctl")
	if err := os.WriteFile(binary, []byte("placeholder"), 0700); err != nil {
		t.Fatal(err)
	}
	s := EtcdctlSource{
		Etcdctl: binary, Etcdutl: binary, Endpoints: []string{"http://etcd:2379"},
		Identity:   Identity{Core: "core-123", Target: "target-123", Cluster: "cluster-123"},
		ScratchDir: root,
	}
	if err := s.validate(); err == nil {
		t.Fatal("non-HTTPS endpoint accepted")
	}
	s.Endpoints = []string{"https://user:password@etcd:2379"}
	if err := s.validate(); err == nil {
		t.Fatal("URL credentials accepted")
	}
}

func TestAuthenticatedEndpointStatusAttestation(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "etcdctl")
	script := "#!/bin/sh\n" +
		"printf '%s' '[{\"Endpoint\":\"https://etcd-1:2379\",\"Status\":{\"header\":{\"cluster_id\":12345,\"revision\":42},\"version\":\"3.6.0\"}}]'\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	source := EtcdctlSource{Etcdctl: binary, Endpoints: []string{"https://etcd-1:2379"}, Identity: Identity{Core: "core-123", Target: "target-123", Cluster: "12345"}}
	info, err := source.attestEndpointStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.ClusterID != "12345" || info.Revision != 42 || info.Version != "3.6.0" {
		t.Fatalf("unexpected attestation: %+v", info)
	}
	source.Identity.Cluster = "foreign"
	if _, err := source.attestEndpointStatus(context.Background()); err == nil {
		t.Fatal("foreign cluster accepted")
	}
}
