package openbao

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotationRecoveryPreflightRejectsUnsafeMaterialWithoutEchoingKeys(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.json")
	data, err := json.Marshal(recoveryBundle{Version: recoveryVersion, KeyShares: 1, KeyThreshold: 1, UnsealKeys: []string{"private-recovery-fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(valid, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRecoveryFile(valid); err != nil {
		t.Fatal("protected valid material rejected", err)
	}
	link := filepath.Join(root, "link.json")
	if err := os.Symlink(valid, link); err != nil {
		t.Fatal(err)
	}
	unsafe := filepath.Join(root, "unsafe.json")
	invalid := filepath.Join(root, "invalid.json")
	oversized := filepath.Join(root, "oversized.json")
	for path, value := range map[string][]byte{unsafe: data, invalid: []byte(`{"unseal_keys":["private-recovery-fixture"]}`), oversized: []byte(strings.Repeat(" ", 64*1024+1))} {
		if err := os.WriteFile(path, value, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(unsafe, 0644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, link, unsafe, invalid, oversized, filepath.Join(root, "absent.json")} {
		err := ValidateRecoveryFile(path)
		if err == nil || strings.Contains(err.Error(), "private-recovery-fixture") || strings.Contains(err.Error(), root) {
			t.Fatal("unsafe recovery material accepted or exposed", err)
		}
	}
}
