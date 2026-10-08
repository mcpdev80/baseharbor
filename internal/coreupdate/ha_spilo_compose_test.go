package coreupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnedSpiloComposeStagePreservesOtherServices(t *testing.T) {
	prior := BackingPin{Role: "core-ha-postgresql", Version: "18-spilo-4.1-p2", Image: "ghcr.io/zalando/spilo-18:4.1-p2", Digest: digestA}
	target := BackingPin{Role: "core-ha-postgresql", Version: "18-spilo-4.2-p1", Image: "ghcr.io/zalando/spilo-18:4.2-p1", Digest: digestB}
	manifest := "services:\n  postgres-member-1:\n    image: " + prior.Image + "\n  postgres-member-2:\n    image: " + prior.Image + "\n  postgres-member-3:\n    image: " + prior.Image + "\n  postgres-etcd-1:\n    image: etcd:stable\n  openbao:\n    image: bao:stable\n"
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	checkpoint := HAPostgresComposeCheckpoint{Path: path, Directory: filepath.Join(dir, "checkpoint"), Previous: prior, Desired: target}
	if err := checkpoint.Stage(); err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(a), target.Image+"@"+target.Digest) != 3 {
		t.Fatalf("not all three Spilo members pinned: %s", a)
	}
	if !strings.Contains(string(a), "etcd:stable") || !strings.Contains(string(a), "bao:stable") {
		t.Fatal("unrelated provider changed")
	}
	if err := checkpoint.Stage(); err != nil {
		t.Fatalf("idempotent stage failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("services: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Stage(); err == nil {
		t.Fatal("foreign manifest drift accepted")
	}
}
func TestOwnedSpiloComposeRejectsMajorChangeAndIncompleteMembers(t *testing.T) {
	a := BackingPin{Role: "core-ha-postgresql", Version: "18-spilo-4.1-p2", Image: "spilo:18", Digest: digestA}
	b := BackingPin{Role: "core-ha-postgresql", Version: "19-spilo-5.0", Image: "spilo:19", Digest: digestB}
	source := []byte("services:\n  postgres-member-1:\n    image: spilo:18\n  postgres-member-2:\n    image: spilo:18\n  postgres-member-3:\n    image: spilo:18\n")
	if _, err := RewriteOwnedSpiloImages(source, a, b); err == nil {
		t.Fatal("PostgreSQL major upgrade accepted")
	}
	b.Version = "18-spilo-4.2"
	if _, err := RewriteOwnedSpiloImages(source, a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := RewriteOwnedSpiloImages([]byte("services:\n  postgres-member-1:\n    image: spilo:18\n"), a, b); err == nil {
		t.Fatal("partial HA topology accepted")
	}
}

func TestSpiloTransitionRejectsDowngradeAndUnknownVersion(t *testing.T) {
 old:=BackingPin{Role:"core-ha-postgresql",Version:"18-spilo-4.2-p2",Image:"spilo:old",Digest:digestA}
 newPin:=BackingPin{Role:"core-ha-postgresql",Version:"18-spilo-4.2-p1",Image:"spilo:new",Digest:digestB}
 manifest:=[]byte("services:\n  postgres-member-1:\n    image: spilo:old\n  postgres-member-2:\n    image: spilo:old\n  postgres-member-3:\n    image: spilo:old\n")
 for _,v:=range []string{"18-spilo-4.2-p1","18-spilo-3.9-p9","18-spilo-5.0-p1","18-spilo-4.3","19-spilo-4.2-p3"} {
  newPin.Version=v
  if _,err:=RewriteOwnedSpiloImages(manifest,old,newPin);err==nil{t.Fatalf("unsafe Spilo transition %q accepted",v)}
 }
 newPin.Version="18-spilo-4.3-p0"
 if _,err:=RewriteOwnedSpiloImages(manifest,old,newPin);err!=nil{t.Fatalf("supported same-major staging rejected: %v",err)}
}
