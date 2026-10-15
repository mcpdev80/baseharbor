package application

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCapabilityRetirementRetainsExistingCredentialAndRecoveryOwner(t *testing.T) {
	for _, provider := range []string{"messaging", "document", "sql", "cache", "object"} {
		t.Run(provider, func(t *testing.T) {
			m := New("retained-env", "dev", false, false, false)
			switch provider {
			case "messaging":
				m.Services.MessagingQueue = true
				m.Services.MessagingManagementUI = true
			case "document":
				m.Services.DocumentDatabase = true
			case "sql":
				m.Services.SQL = true
			case "cache":
				m.Services.Cache = true
			case "object":
				m.Services.ObjectStorage = true
			}
			path := filepath.Join(t.TempDir(), "runtime.env")
			if err := ensureRuntimeEnv(path, m); err != nil {
				t.Fatal(err)
			}
			before, err := readRuntimeEnv(path)
			if err != nil || len(before) == 0 {
				t.Fatal("original provider credential owner missing")
			}
			// Unknown protected recovery material also remains in the same owner;
			// it does not become an undeclared workload environment variable.
			before["RETAINED_RECOVERY_REFERENCE"] = "protected-owner-reference"
			if err := writeRuntimeEnv(path, m, before); err != nil {
				t.Fatal(err)
			}
			inactive := m
			inactive.Services = Services{}
			if err := ensureRuntimeEnv(path, inactive); err != nil {
				t.Fatal(err)
			}
			retained, err := readRuntimeEnv(path)
			if err != nil || !reflect.DeepEqual(before, retained) {
				t.Fatal("capability retirement discarded credential or recovery values")
			}
			projected, err := containerRuntimeEnvironment(inactive, retained)
			if err != nil || len(projected) != 0 {
				t.Fatal("inactive protected credentials entered workload projection")
			}
			if resources := ExpectedRuntimeResources(inactive); len(resources) != 0 {
				t.Fatal("retained credentials enabled undeclared runtime")
			}
			if err := ensureRuntimeEnv(path, m); err != nil {
				t.Fatal(err)
			}
			after, err := readRuntimeEnv(path)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("re-add replaced the retained credential/recovery identity")
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("retained credential owner is not protected")
			}
		})
	}
}

func TestInvalidRetainedEnvironmentCannotBeRewrittenOrDisclosed(t *testing.T) {
	for _, entry := range []map[string]string{
		{"INVALID=sentinel-credential": "value"},
		{"RETAINED_PASSWORD": "sentinel-credential\nINJECTED=value"},
		{"RETAINED_PASSWORD": "sentinel-credential\rvalue"},
		{"RETAINED_PASSWORD": "sentinel-credential\x00value"},
	} {
		path := filepath.Join(t.TempDir(), "runtime.env")
		original := []byte("RETAINED_PASSWORD=unchanged-protected-value\n")
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		err := writeRuntimeEnv(path, New("retained-env", "dev", false, false, false), entry)
		if err == nil || strings.Contains(err.Error(), "sentinel-credential") {
			t.Fatal("invalid retained entry accepted or echoed")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(original) {
			t.Fatal("invalid retained material rewrote the original owner")
		}
	}
}
