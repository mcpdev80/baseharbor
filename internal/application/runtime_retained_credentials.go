package application

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/providertopology"
)

// Reconciliation may initialize a new independent credential group. Once its
// existing protected owner contains material for that group, missing values
// require recovery rather than implicit regeneration. Rotation remains in its
// existing explicit provider operation, not normal application reapply.
func validateRetainedRuntimeCredentials(path string, values map[string]string, m Manifest) error {
	if len(SQLInstanceNames(m))+len(ValkeyInstanceNames(m))+len(RabbitMQInstanceNames(m))+len(DocumentDatabaseInstanceNames(m))+len(ObjectStorageBucketNames(m)) == 0 && !m.Services.SQLManagementUI && !m.Services.CacheManagementUI && !m.Services.KeyValueManagementUI && !m.Services.DocumentDatabaseManagementUI {
		return nil // No credential generation: preserve deliberate retirement.
	}
	services, err := providertopology.ServiceNames(filepath.Join(filepath.Dir(path), "compose.yaml"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("retained provider inventory is unverifiable; original credentials were not regenerated")
	}
	retained := func(kind, instance string) bool {
		if _, exists := services[runtimeServiceName(kind, instance)]; exists {
			return true
		}
		root := filepath.Join(filepath.Dir(path), "bindings", kind)
		if instance != defaultServiceInstance {
			_, err := os.Lstat(filepath.Join(root, instance))
			return !errors.Is(err, os.ErrNotExist)
		}
		// A kind directory can contain only other named instances. Its direct
		// material/metadata files establish the old default binding instead.
		for _, file := range []string{"type", "host", "bucket", "password", "access_key_id", "secret_access_key", "certificates"} {
			_, err := os.Lstat(filepath.Join(root, file))
			if !errors.Is(err, os.ErrNotExist) {
				return true
			}
		}
		_, err := os.Lstat(filepath.Join(root, defaultServiceInstance))
		return !errors.Is(err, os.ErrNotExist)
	}
	check := func(present bool, required []string, signals ...string) error {
		for _, key := range append(append([]string{}, required...), signals...) {
			_, exists := values[key]
			present = present || exists
		}
		if !present {
			return nil
		}
		for _, key := range required {
			if strings.TrimSpace(values[key]) == "" {
				return fmt.Errorf("protected application credential material is incomplete (%s); restore its original credential/recovery owner before retrying; no retained credentials were regenerated", key)
			}
		}
		return nil
	}
	for _, instance := range SQLInstanceNames(m) {
		if err := check(retained("postgres", instance), []string{postgresRuntimeKey(instance, "DB"), postgresRuntimeKey(instance, "USER"), postgresRuntimeKey(instance, "PASSWORD")}, postgresRuntimeKey(instance, "HOST_PORT"), postgresRuntimeKey(instance, "TLS_CA_FILE"), postgresRuntimeKey(instance, "CONTAINER_HOST")); err != nil {
			return err
		}
	}
	for _, instance := range ValkeyInstanceNames(m) {
		if err := check(retained("valkey", instance), []string{valkeyRuntimeKey(instance, "PASSWORD")}, valkeyRuntimeKey(instance, "HOST_PORT"), valkeyRuntimeKey(instance, "USER"), valkeyRuntimeKey(instance, "TLS_CA_FILE"), valkeyRuntimeKey(instance, "CONTAINER_HOST")); err != nil {
			return err
		}
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		keys := []string{rabbitmqRuntimeKey(instance, "USER"), rabbitmqRuntimeKey(instance, "PASSWORD"), rabbitmqRuntimeKey(instance, "BOOTSTRAP_USER"), rabbitmqRuntimeKey(instance, "BOOTSTRAP_PASSWORD")}
		if rabbitmqMemberCount(m) > 1 {
			keys = append(keys, rabbitmqRuntimeKey(instance, "ERLANG_COOKIE"))
		}
		if err := check(retained("rabbitmq", instance), keys, rabbitmqRuntimeKey(instance, "HOST_PORT"), rabbitmqRuntimeKey(instance, "TLS_CA_FILE"), rabbitmqContainerHostKey(instance)); err != nil {
			return err
		}
		if m.Services.MessagingManagementUI {
			_, exists := services[rabbitmqUIServiceName(instance)]
			if err := check(exists, []string{rabbitmqRuntimeKey(instance, "ADMIN_USER"), rabbitmqRuntimeKey(instance, "ADMIN_PASSWORD")}, rabbitmqUIHostPortKey(instance)); err != nil {
				return err
			}
		}
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		keys := []string{mongodbRuntimeKey(instance, "DB"), mongodbRuntimeKey(instance, "USER"), mongodbRuntimeKey(instance, "PASSWORD"), mongodbRuntimeKey(instance, "ADMIN_USER"), mongodbRuntimeKey(instance, "ADMIN_PASSWORD")}
		if mongodbMemberCount(m, instance) > 1 {
			keys = append(keys, mongodbReplicaSetKey(instance), mongodbReplicaKeyKey(instance))
		}
		if err := check(retained("mongodb", instance), keys, mongodbRuntimeKey(instance, "HOST_PORT"), mongodbContainerHostKey(instance), mongodbRuntimeKey(instance, "TLS_CA_FILE")); err != nil {
			return err
		}
	}
	for _, bucket := range ObjectStorageBucketNames(m) {
		if err := check(retained("object-storage-s3", bucket), []string{s3RuntimeKey(bucket, "ACCESS_KEY_ID"), s3RuntimeKey(bucket, "SECRET_ACCESS_KEY")}); err != nil {
			return err
		}
	}
	if m.Services.SQLManagementUI {
		_, exists := services["postgres-ui"]
		if err := check(exists, []string{PostgresUIEmailEnv, PostgresUIPasswordEnv}, PostgresUIHostPortEnv); err != nil {
			return err
		}
	}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		_, exists := services["cache-ui"]
		if err := check(exists, []string{CacheUIUserEnv, CacheUIPasswordEnv}, CacheUIHostPortEnv); err != nil {
			return err
		}
	}
	if m.Services.DocumentDatabaseManagementUI {
		ports := []string{}
		exists := false
		for _, instance := range DocumentDatabaseInstanceNames(m) {
			ports = append(ports, mongodbUIHostPortKey(instance))
			_, found := services[mongodbUIServiceName(instance)]
			exists = exists || found
		}
		if err := check(exists, []string{MongoDBUIUserEnv, MongoDBUIPasswordEnv}, ports...); err != nil {
			return err
		}
	}
	return nil
}
