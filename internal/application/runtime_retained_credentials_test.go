package application

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReapplyDoesNotRegenerateMissingRetainedPassword(t *testing.T) {
	m := New("retained-auth", "dev", true, false, false)
	path := filepath.Join(t.TempDir(), "runtime.env")
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	values, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	delete(values, postgresRuntimeKey(defaultServiceInstance, "PASSWORD"))
	var broken strings.Builder
	for key, value := range values {
		broken.WriteString(key + "=" + value + "\n")
	}
	before := []byte(broken.String())
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, m); err == nil {
		t.Fatal("reapply regenerated a missing retained credential")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed continuation rewrote the credential owner")
	}
}

func TestRetainedCredentialGroupsDoNotRegenerateAcrossProviderFamilies(t *testing.T) {
	for _, family := range []string{"sql", "cache", "messaging", "messaging-ui", "messaging-ha", "document", "document-ui", "document-ha", "object", "sql-ui", "cache-ui", "named-sql", "named-cache", "named-messaging", "named-document", "named-object"} {
		t.Run(family, func(t *testing.T) {
			m := retainedCredentialFixture(family)
			path := filepath.Join(t.TempDir(), "runtime.env")
			if err := ensureRuntimeEnv(path, m); err != nil {
				t.Fatal(err)
			}
			original, err := readRuntimeEnv(path)
			if err != nil {
				t.Fatal(err)
			}
			for key, value := range original {
				if value == "" || !(strings.HasSuffix(key, "PASSWORD") || strings.HasSuffix(key, "SECRET_ACCESS_KEY") || strings.HasSuffix(key, "ACCESS_KEY_ID") || strings.HasSuffix(key, "USER") || strings.HasSuffix(key, "_DB") || strings.HasSuffix(key, "ERLANG_COOKIE") || strings.HasSuffix(key, "REPLICA_KEY") || key == PostgresUIEmailEnv) {
					continue
				}
				for _, empty := range []bool{false, true} {
					damaged := make(map[string]string, len(original))
					for k, v := range original {
						damaged[k] = v
					}
					if empty {
						damaged[key] = ""
					} else {
						delete(damaged, key)
					}
					var b strings.Builder
					for k, v := range damaged {
						b.WriteString(k + "=" + v + "\n")
					}
					before := b.String()
					if err := os.WriteFile(path, []byte(before), 0600); err != nil {
						t.Fatal(err)
					}
					err := ensureRuntimeEnv(path, m)
					if err == nil {
						t.Fatalf("missing credential was regenerated: %s", key)
					}
					for _, secret := range original {
						if len(secret) >= 20 && strings.Contains(err.Error(), secret) {
							t.Fatal("credential value entered error output")
						}
					}
					after, readErr := os.ReadFile(path)
					if readErr != nil || string(after) != before {
						t.Fatal("rejected continuation rewrote retained owner")
					}
				}
			}
		})
	}
}

func TestEnsureRuntimeRejectsLostCredentialsBeforeIssuanceOrProjection(t *testing.T) {
	m := New("retained-auth", "dev", true, false, false)
	store := Store{Root: t.TempDir()}
	files := RuntimeFilesFor(store, m)
	if err := os.MkdirAll(files.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	before := []byte("POSTGRES_DB=original_data\nPOSTGRES_USER=original_user\nPOSTGRES_HOST_PORT=5432\n")
	if err := os.WriteFile(files.Env, before, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureRuntime(context.Background(), nil, store, m)
	if err == nil || !strings.Contains(err.Error(), "credential material is incomplete") {
		t.Fatal("lost credentials did not fail before issuer/projection work")
	}
	after, err := os.ReadFile(files.Env)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed preparation rewrote the original owner")
	}
	entries, err := os.ReadDir(files.Dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "runtime.env" {
		t.Fatal("failed preparation created runtime, trust or binding projections")
	}
}

func retainedCredentialFixture(family string) Manifest {
	m := New("retained-auth", "dev", false, false, false)
	switch family {
	case "sql", "sql-ui":
		m.Services.SQL, m.Services.SQLManagementUI = true, family == "sql-ui"
	case "cache", "cache-ui":
		m.Services.Cache, m.Services.CacheManagementUI = true, family == "cache-ui"
	case "messaging", "messaging-ui", "messaging-ha":
		m.Services.MessagingQueue, m.Services.MessagingManagementUI, m.HA = true, family == "messaging-ui", family == "messaging-ha"
	case "document", "document-ui", "document-ha":
		m.Services.DocumentDatabase, m.Services.DocumentDatabaseManagementUI, m.HA = true, family == "document-ui", family == "document-ha"
	case "object":
		m.Services.ObjectStorage = true
	case "named-sql":
		m = WithSQLInstances(m, "analytics")
	case "named-cache":
		m = WithCacheInstances(m, "sessions")
	case "named-messaging":
		m = WithMessagingQueueInstances(m, "events")
	case "named-document":
		m = WithDocumentDatabaseInstances(m, "catalog")
	case "named-object":
		m = WithObjectStorageBuckets(m, "uploads")
	}
	return m
}

func TestAddingIndependentCredentialsPreservesExistingMaterial(t *testing.T) {
	m := WithSQLInstances(New("retained-auth", "dev", false, false, false), "primary")
	path := filepath.Join(t.TempDir(), "runtime.env")
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	before, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	m = WithSQLInstances(m, "primary", "analytics")
	m.Services.Cache, m.Services.MessagingQueue, m.Services.DocumentDatabase, m.Services.ObjectStorage = true, true, true, true
	m.Services.SQLManagementUI, m.Services.MessagingManagementUI, m.Services.CacheManagementUI, m.Services.DocumentDatabaseManagementUI = true, true, true, true
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	after, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range before {
		if after[key] != value {
			t.Fatalf("adding independent credentials changed existing key: %s", key)
		}
	}
	if after[postgresRuntimeKey("analytics", "PASSWORD")] == "" {
		t.Fatal("new independent instance did not receive its initial credential")
	}
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	repeated, err := readRuntimeEnv(path)
	if err != nil || !reflect.DeepEqual(after, repeated) {
		t.Fatal("repeat apply changed credentials")
	}
}

func TestRetiringDamagedCredentialsDoesNotPreventIntentionalRemoval(t *testing.T) {
	m := New("retained-auth", "dev", true, false, false)
	path := filepath.Join(t.TempDir(), "runtime.env")
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	values, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	values[postgresRuntimeKey(defaultServiceInstance, "PASSWORD")] = ""
	if err := writeRuntimeEnv(path, m, values); err != nil {
		t.Fatal(err)
	}
	m.Services.SQL = false
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "compose.yaml"), []byte("damaged previous inventory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal("retiring damaged credentials must remain possible")
	}
	after, err := readRuntimeEnv(path)
	if err != nil || !reflect.DeepEqual(values, after) {
		t.Fatal("removal replaced retained credential material")
	}
}

func TestExistingNativeOwnerPreventsRegenerationWhenEntireCredentialGroupIsMissing(t *testing.T) {
	for _, family := range []string{"sql", "cache", "messaging", "document", "object", "named-sql", "named-cache", "named-messaging", "named-document", "named-object"} {
		for _, missingFile := range []bool{false, true} {
			t.Run(family+map[bool]string{false: "-missing-group", true: "-missing-file"}[missingFile], func(t *testing.T) {
				m := retainedCredentialFixture(family)
				root := t.TempDir()
				path := filepath.Join(root, "runtime.env")
				kind, instance := "", defaultServiceInstance
				switch family {
				case "sql", "named-sql":
					kind = "postgres"
					if family == "named-sql" {
						instance = "analytics"
					}
				case "cache", "named-cache":
					kind = "valkey"
					if family == "named-cache" {
						instance = "sessions"
					}
				case "messaging", "named-messaging":
					kind = "rabbitmq"
					if family == "named-messaging" {
						instance = "events"
					}
				case "document", "named-document":
					kind = "mongodb"
					if family == "named-document" {
						instance = "catalog"
					}
				case "object", "named-object":
					kind = "object-storage-s3"
					if family == "named-object" {
						instance = "uploads"
					}
				}
				if kind == "object-storage-s3" {
					binding := filepath.Join(root, "bindings", kind)
					if instance != defaultServiceInstance {
						binding = filepath.Join(binding, instance)
					}
					if err := os.MkdirAll(binding, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(binding, "bucket"), []byte("owned-bucket\n"), 0600); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(filepath.Join(root, "compose.yaml"), []byte("services:\n  "+runtimeServiceName(kind, instance)+": {}\n"), 0600); err != nil {
					t.Fatal(err)
				}
				before := []byte("RETAINED_RECOVERY_REFERENCE=unchanged-owner\n")
				if !missingFile {
					if err := os.WriteFile(path, before, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := ensureRuntimeEnv(path, m); err == nil {
					t.Fatal("lost credentials were recreated for an existing provider owner")
				}
				after, err := os.ReadFile(path)
				if missingFile {
					if !os.IsNotExist(err) {
						t.Fatal("missing original owner was silently recreated")
					}
				} else if err != nil || string(after) != string(before) {
					t.Fatal("rejected continuation rewrote retained owner")
				}
			})
		}
	}
}

func TestNewDefaultBindingDoesNotAdoptExistingNamedCredentialGroup(t *testing.T) {
	m := WithObjectStorageBuckets(New("retained-auth", "dev", false, false, false), "uploads")
	root := t.TempDir()
	path := filepath.Join(root, "runtime.env")
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal(err)
	}
	before, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bindings", "object-storage-s3", "uploads"), 0700); err != nil {
		t.Fatal(err)
	}
	m = WithObjectStorageBuckets(m, "uploads", defaultServiceInstance)
	if err := ensureRuntimeEnv(path, m); err != nil {
		t.Fatal("new independent default binding was mistaken for the named owner", err)
	}
	after, err := readRuntimeEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range before {
		if after[key] != value {
			t.Fatal("adding default binding replaced named credential material")
		}
	}
}
