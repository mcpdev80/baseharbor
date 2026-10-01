package application

import (
	"fmt"
	"strconv"
	"strings"
)

const MongoDBImage = "docker.io/library/mongo:8.3.11"

func mongodbRuntimeKey(instance, suffix string) string {
	return runtimeInstanceKey("MONGODB", instance, suffix)
}

func mongodbContainerHostKey(instance string) string {
	return mongodbRuntimeKey(instance, "CONTAINER_HOST")
}

func mongodbDatabaseName(m Manifest, instance string) string {
	base := strings.ReplaceAll(m.Name+"_"+m.Environment, "-", "_")
	if instance == defaultServiceInstance {
		return base
	}
	return base + "_" + strings.ReplaceAll(instance, "-", "_")
}

func ensureMongoDBRuntimeValues(values map[string]string, m Manifest, excluded map[int]struct{}) error {
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		dbKey := mongodbRuntimeKey(instance, "DB")
		userKey := mongodbRuntimeKey(instance, "USER")
		passwordKey := mongodbRuntimeKey(instance, "PASSWORD")
		portKey := mongodbRuntimeKey(instance, "HOST_PORT")
		if values[dbKey] == "" {
			values[dbKey] = mongodbDatabaseName(m, instance)
		}
		if values[userKey] == "" {
			values[userKey] = "baseharbor"
		}
		if values[passwordKey] == "" {
			password, err := randomApplicationSecret(32)
			if err != nil {
				return err
			}
			values[passwordKey] = password
		}
		if values[portKey] == "" {
			port, err := allocateLoopbackPort(excluded)
			if err != nil {
				return err
			}
			values[portKey] = strconv.Itoa(port)
			excluded[port] = struct{}{}
		}
	}
	return nil
}

func appendMongoDBRuntimeEnv(b *strings.Builder, m Manifest, values map[string]string) {
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		for _, suffix := range []string{"DB", "USER", "PASSWORD", "HOST_PORT", "TLS_CA_FILE", "CONTAINER_HOST"} {
			key := mongodbRuntimeKey(instance, suffix)
			fmt.Fprintf(b, "%s=%s\n", key, values[key])
		}
	}
}

func validateMongoDBRuntimeValues(values map[string]string, m Manifest) error {
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		for _, suffix := range []string{"DB", "USER", "PASSWORD", "HOST_PORT"} {
			key := mongodbRuntimeKey(instance, suffix)
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		portKey := mongodbRuntimeKey(instance, "HOST_PORT")
		if err := validatePortValue(values[portKey], portKey); err != nil {
			return err
		}
	}
	return nil
}

func writeMongoDBComposeService(b *strings.Builder, instance string) {
	service := runtimeServiceName("mongodb", instance)
	userKey := mongodbRuntimeKey(instance, "USER")
	passwordKey := mongodbRuntimeKey(instance, "PASSWORD")
	fmt.Fprintf(b, `  %s:
    image: %s
    restart: unless-stopped
    read_only: true
    cap_drop: ["ALL"]
    cap_add: ["CHOWN", "SETGID", "SETUID"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    environment:
      MONGO_INITDB_ROOT_USERNAME: ${%s}
      MONGO_INITDB_ROOT_PASSWORD: ${%s}
    volumes:
      - %s-data:/data/db
    healthcheck:
      test: ["CMD-SHELL", "mongosh --quiet --username \"$$MONGO_INITDB_ROOT_USERNAME\" --password \"$$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --eval 'quit(db.adminCommand({ ping: 1 }).ok ? 0 : 2)'"]
      interval: 5s
      timeout: 10s
      retries: 18
      start_period: 15s

`, service, MongoDBImage, userKey, passwordKey, service)
}
