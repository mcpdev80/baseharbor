package application

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCoreSharedValkeyACLReadableByContainerUser(t *testing.T) {
	dir := t.TempDir()
	adminCredential, err := ensureSharedValkeyCredential(dir, "provider-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	shared := SharedBackendFiles{Dir: dir}
	state := sharedBackendState{ValkeyAdminCredential: adminCredential, Applications: map[string]sharedBackendAppState{}}
	if err := writeCoreSharedValkeyACL(shared, state); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "valkey", "core", defaultServiceInstance, "consumer-acl.conf")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0644 {
		t.Fatalf("ACL mode = %04o, want 0644 so the numeric container user can read the bind mount", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("ACL directory mode = %04o, want 0700 to keep the hash file private on the host", got)
	}
}
