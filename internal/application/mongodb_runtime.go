package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const MongoDBImage = "docker.io/library/mongo:7.0.43"

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
		adminUserKey := mongodbRuntimeKey(instance, "ADMIN_USER")
		adminPasswordKey := mongodbRuntimeKey(instance, "ADMIN_PASSWORD")
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
		if values[adminUserKey] == "" {
			values[adminUserKey] = "baseharbor_admin"
		}
		if values[adminPasswordKey] == "" {
			password, err := randomApplicationSecret(32)
			if err != nil {
				return err
			}
			values[adminPasswordKey] = password
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
		for _, suffix := range []string{"DB", "USER", "PASSWORD", "ADMIN_USER", "ADMIN_PASSWORD", "HOST_PORT", "TLS_CA_FILE", "CONTAINER_HOST"} {
			key := mongodbRuntimeKey(instance, suffix)
			fmt.Fprintf(b, "%s=%s\n", key, values[key])
		}
	}
}

func validateMongoDBRuntimeValues(values map[string]string, m Manifest) error {
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		for _, suffix := range []string{"DB", "USER", "PASSWORD", "ADMIN_USER", "ADMIN_PASSWORD", "HOST_PORT"} {
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
	dbKey := mongodbRuntimeKey(instance, "DB")
	userKey := mongodbRuntimeKey(instance, "USER")
	passwordKey := mongodbRuntimeKey(instance, "PASSWORD")
	adminUserKey := mongodbRuntimeKey(instance, "ADMIN_USER")
	adminPasswordKey := mongodbRuntimeKey(instance, "ADMIN_PASSWORD")
	initScript := "./" + filepath.ToSlash(filepath.Join("providers", "mongodb", instance, "init.js"))
	fmt.Fprintf(b, `  %s:
    image: %s
    restart: unless-stopped
    read_only: true
    cap_drop: ["ALL"]
    cap_add: ["CHOWN", "SETGID", "SETUID"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /data/configdb:rw,noexec,nosuid,nodev
    environment:
      MONGO_INITDB_ROOT_USERNAME: ${%s}
      MONGO_INITDB_ROOT_PASSWORD: ${%s}
      MONGO_INITDB_DATABASE: ${%s}
      BASEHARBOR_MONGODB_USER: ${%s}
      BASEHARBOR_MONGODB_PASSWORD: ${%s}
    volumes:
      - %s-data:/data/db
      - %s:/docker-entrypoint-initdb.d/10-baseharbor-app-user.js:ro
    healthcheck:
      test: ["CMD-SHELL", "mongosh --quiet --username \"$$MONGO_INITDB_ROOT_USERNAME\" --password \"$$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --eval 'quit(db.adminCommand({ ping: 1 }).ok ? 0 : 2)'"]
      interval: 5s
      timeout: 10s
      retries: 18
      start_period: 15s

`, service, MongoDBImage, adminUserKey, adminPasswordKey, dbKey, userKey, passwordKey, service, initScript)
}

func ensureMongoDBInitFiles(files RuntimeFiles, m Manifest) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		dir := filepath.Join(files.Dir, "providers", "mongodb", instance)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create MongoDB provider directory: %w", err)
		}
		db, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "DB"))
		if err != nil {
			return err
		}
		user, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "USER"))
		if err != nil {
			return err
		}
		password, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "PASSWORD"))
		if err != nil {
			return err
		}
		script := fmt.Sprintf(
			"const appdb = db.getSiblingDB(%s);\nappdb.createUser({user:%s,pwd:%s,roles:[{role:\"readWrite\",db:%s}]});\n",
			strconv.Quote(db), strconv.Quote(user), strconv.Quote(password), strconv.Quote(db),
		)
		if err := writeOwnerOnlyFile(filepath.Join(dir, "init.js"), []byte(script)); err != nil {
			return fmt.Errorf("write MongoDB application-user init script: %w", err)
		}
	}
	return nil
}
