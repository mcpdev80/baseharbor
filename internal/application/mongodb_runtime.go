package application

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
)

const MongoDBImage = "docker.io/library/mongo:7.0.43"

func mongodbRuntimeKey(instance, suffix string) string {
	return runtimeInstanceKey("MONGODB", instance, suffix)
}

func randomMongoDBReplicaKey(size int) (string, error) {
	if size < 6 {
		return "", fmt.Errorf("MongoDB replica key size must be at least 6 bytes")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate MongoDB replica key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
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
	if m.Services.DocumentDatabaseManagementUI {
		if values[MongoDBUIUserEnv] == "" {
			values[MongoDBUIUserEnv] = "baseharbor"
		}
		if values[MongoDBUIPasswordEnv] == "" {
			password, err := randomApplicationSecret(24)
			if err != nil {
				return err
			}
			values[MongoDBUIPasswordEnv] = password
		}
	}
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
		if mongodbMemberCount(m, instance) > 1 {
			if values[mongodbReplicaSetKey(instance)] == "" {
				values[mongodbReplicaSetKey(instance)] = mongodbReplicaSetName(instance)
			}
			if values[mongodbReplicaKeyKey(instance)] == "" {
				key, err := randomMongoDBReplicaKey(64)
				if err != nil {
					return err
				}
				values[mongodbReplicaKeyKey(instance)] = key
			}
			for ordinal := 1; ordinal < mongodbMemberCount(m, instance); ordinal++ {
				memberPortKey := mongodbMemberHostPortKey(instance, ordinal)
				if values[memberPortKey] == "" {
					port, err := allocateLoopbackPort(excluded)
					if err != nil {
						return err
					}
					values[memberPortKey] = strconv.Itoa(port)
					excluded[port] = struct{}{}
				}
			}
		}
		if m.Services.DocumentDatabaseManagementUI {
			uiPortKey := mongodbUIHostPortKey(instance)
			if values[uiPortKey] == "" {
				port, err := allocateLoopbackPort(excluded)
				if err != nil {
					return err
				}
				values[uiPortKey] = strconv.Itoa(port)
				excluded[port] = struct{}{}
			}
		}
	}
	return nil
}

func appendMongoDBRuntimeEnv(b *strings.Builder, m Manifest, values map[string]string) {
	if m.Services.DocumentDatabaseManagementUI {
		for _, key := range []string{MongoDBUIUserEnv, MongoDBUIPasswordEnv} {
			fmt.Fprintf(b, "%s=%s\n", key, values[key])
		}
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		suffixes := []string{"DB", "USER", "PASSWORD", "ADMIN_USER", "ADMIN_PASSWORD", "HOST_PORT", "TLS_CA_FILE", "CONTAINER_HOST"}
		if mongodbMemberCount(m, instance) > 1 {
			suffixes = append(suffixes, "REPLICA_SET", "REPLICA_KEY")
		}
		for _, suffix := range suffixes {
			key := mongodbRuntimeKey(instance, suffix)
			fmt.Fprintf(b, "%s=%s\n", key, values[key])
		}
		for ordinal := 1; ordinal < mongodbMemberCount(m, instance); ordinal++ {
			key := mongodbMemberHostPortKey(instance, ordinal)
			fmt.Fprintf(b, "%s=%s\n", key, values[key])
		}
		if m.Services.DocumentDatabaseManagementUI {
			key := mongodbUIHostPortKey(instance)
			fmt.Fprintf(b, "%s=%s\n", key, values[key])
		}
	}
}

func validateMongoDBRuntimeValues(values map[string]string, m Manifest) error {
	if m.Services.DocumentDatabaseManagementUI {
		for _, key := range []string{MongoDBUIUserEnv, MongoDBUIPasswordEnv} {
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
	}
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
		if mongodbMemberCount(m, instance) > 1 {
			for _, key := range []string{mongodbReplicaSetKey(instance), mongodbReplicaKeyKey(instance)} {
				if values[key] == "" {
					return fmt.Errorf("application runtime environment is missing %s", key)
				}
			}
			for ordinal := 1; ordinal < mongodbMemberCount(m, instance); ordinal++ {
				key := mongodbMemberHostPortKey(instance, ordinal)
				if values[key] == "" {
					return fmt.Errorf("application runtime environment is missing %s", key)
				}
				if err := validatePortValue(values[key], key); err != nil {
					return err
				}
			}
		}
		if m.Services.DocumentDatabaseManagementUI {
			uiPortKey := mongodbUIHostPortKey(instance)
			if values[uiPortKey] == "" {
				return fmt.Errorf("application runtime environment is missing %s", uiPortKey)
			}
			if err := validatePortValue(values[uiPortKey], uiPortKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeMongoDBComposeService(b *strings.Builder, m Manifest, instance string) {
	dbKey := mongodbRuntimeKey(instance, "DB")
	userKey := mongodbRuntimeKey(instance, "USER")
	passwordKey := mongodbRuntimeKey(instance, "PASSWORD")
	adminUserKey := mongodbRuntimeKey(instance, "ADMIN_USER")
	adminPasswordKey := mongodbRuntimeKey(instance, "ADMIN_PASSWORD")
	replicaSetKey := mongodbReplicaSetKey(instance)
	replicaKeyKey := mongodbReplicaKeyKey(instance)
	initScript := "./" + filepath.ToSlash(filepath.Join("providers", "mongodb", instance, "init.js"))
	for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
		service := mongodbMemberServiceName(instance, ordinal)
		hostPortKey := mongodbMemberHostPortKey(instance, ordinal)
		fmt.Fprintf(b, "  %s:\n", service)
		fmt.Fprintf(b, "    image: %s\n", MongoDBImage)
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    cap_add: [\"CHOWN\", \"DAC_OVERRIDE\", \"SETGID\", \"SETUID\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n      - /data/configdb:rw,noexec,nosuid,nodev\n")
		b.WriteString("    ports:\n")
		fmt.Fprintf(b, "      - \"127.0.0.1:${%s}:27017\"\n", hostPortKey)
		b.WriteString("    environment:\n")
		fmt.Fprintf(b, "      MONGO_INITDB_ROOT_USERNAME: ${%s}\n", adminUserKey)
		fmt.Fprintf(b, "      MONGO_INITDB_ROOT_PASSWORD: ${%s}\n", adminPasswordKey)
		fmt.Fprintf(b, "      MONGO_INITDB_DATABASE: ${%s}\n", dbKey)
		fmt.Fprintf(b, "      BASEHARBOR_MONGODB_USER: ${%s}\n", userKey)
		fmt.Fprintf(b, "      BASEHARBOR_MONGODB_PASSWORD: ${%s}\n", passwordKey)
		if mongodbMemberCount(m, instance) > 1 {
			fmt.Fprintf(b, "      BASEHARBOR_MONGODB_REPLICA_SET: ${%s}\n", replicaSetKey)
			fmt.Fprintf(b, "      BASEHARBOR_MONGODB_REPLICA_KEY: ${%s}\n", replicaKeyKey)
			b.WriteString("    entrypoint: [\"/bin/sh\", \"-ec\"]\n")
			b.WriteString("    command:\n      - |\n")
			b.WriteString("        printf '%s\\n' \"- \\\"$$BASEHARBOR_MONGODB_REPLICA_KEY\\\"\" > /tmp/mongodb-keyfile\n")
			b.WriteString("        chmod 0400 /tmp/mongodb-keyfile\n")
			b.WriteString("        chown mongodb:mongodb /tmp/mongodb-keyfile\n")
			b.WriteString("        exec /usr/local/bin/docker-entrypoint.sh mongod --bind_ip_all --replSet \"$$BASEHARBOR_MONGODB_REPLICA_SET\" --keyFile /tmp/mongodb-keyfile --tlsMode requireTLS --tlsCertificateKeyFile /run/baseharbor/tls/server.pem --tlsCAFile /run/baseharbor/tls/ca.pem --tlsAllowConnectionsWithoutCertificates --setParameter tlsWithholdClientCertificate=true\n")
		} else {
			b.WriteString("    command: [\"mongod\", \"--bind_ip_all\", \"--tlsMode\", \"requireTLS\", \"--tlsCertificateKeyFile\", \"/run/baseharbor/tls/server.pem\", \"--tlsCAFile\", \"/run/baseharbor/tls/ca.pem\", \"--tlsAllowConnectionsWithoutCertificates\"]\n")
		}
		b.WriteString("    volumes:\n")
		fmt.Fprintf(b, "      - %s:/data/db\n", mongodbMemberVolumeName(instance, ordinal))
		fmt.Fprintf(b, "      - %s:/docker-entrypoint-initdb.d/10-baseharbor-app-user.js:ro\n", initScript)
		fmt.Fprintf(b, "      - ./providers/mongodb/%s/runtime/server.pem:/run/baseharbor/tls/server.pem:ro\n", instance)
		fmt.Fprintf(b, "      - ./providers/mongodb/%s/runtime/ca.pem:/run/baseharbor/tls/ca.pem:ro\n", instance)
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD-SHELL\", \"mongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username \\\"$${MONGO_INITDB_ROOT_USERNAME}\\\" --password \\\"$${MONGO_INITDB_ROOT_PASSWORD}\\\" --authenticationDatabase admin --eval 'quit(db.adminCommand({ ping: 1 }).ok ? 0 : 2)'\"]\n")
		b.WriteString("      interval: 5s\n      timeout: 10s\n      retries: 18\n      start_period: 15s\n\n")
	}
}
func writeMongoDBUIComposeServices(b *strings.Builder, m Manifest, instance string) {
	uiService := mongodbUIServiceName(instance)
	accessService := mongodbUIAccessServiceName(instance)
	var mongoHosts []string
	for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
		mongoHosts = append(mongoHosts, mongodbMemberServiceName(instance, ordinal)+":27017")
	}
	replicaQuery := ""
	if mongodbMemberCount(m, instance) > 1 {
		replicaQuery = "&replicaSet=${" + mongodbReplicaSetKey(instance) + "}"
	}
	dbKey := mongodbRuntimeKey(instance, "DB")
	userKey := mongodbRuntimeKey(instance, "USER")
	passwordKey := mongodbRuntimeKey(instance, "PASSWORD")
	uiPortKey := mongodbUIHostPortKey(instance)
	routeName := mongodbUIRouteName(instance)
	fmt.Fprintf(b, `  %s:
    image: %s
    restart: unless-stopped
    user: "node"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    environment:
      MONGOKU_DEFAULT_HOST: "mongodb://${%s}:${%s}@%s/${%s}?authSource=${%s}&tls=true&tlsCAFile=/run/baseharbor/mongodb-ca.pem%s"
      MONGOKU_SERVER_PROTOCOL_HEADER: "x-forwarded-proto"
      MONGOKU_SERVER_HOST_HEADER: "x-forwarded-host"
      MONGOKU_DATABASE_FILE: "/tmp/mongoku.db"
    volumes:
      - ./bindings/mongodb/%s/ca.pem:/run/baseharbor/mongodb-ca.pem:ro

  %s:
    image: %s
    restart: unless-stopped
    user: "65532:65532"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    entrypoint: ["/bin/sh", "-ec"]
    command:
      - cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /run/baseharbor:rw,exec,nosuid,nodev,mode=1777
      - /data:rw,noexec,nosuid,nodev,mode=1777
      - /config:rw,noexec,nosuid,nodev,mode=1777
    ports:
      - "127.0.0.1:${%s}:8443"
    volumes:
      - ./providers/management-ui/mongodb/%s/Caddyfile:/etc/caddy/Caddyfile:ro
      - ./providers/management-ui/mongodb/%s/server.pem:/certs/server.pem:ro
      - ./providers/management-ui/mongodb/%s/server-key.pem:/certs/server-key.pem:ro
    networks:
      default:
        aliases:
          - %q

`, uiService, MongoDBUIImage, userKey, passwordKey, strings.Join(mongoHosts, ","), dbKey, dbKey, replicaQuery, instance,
		accessService, UIProxyImage, uiPortKey, instance, instance, instance, devaccess.ApplicationAlias(m.Name, routeName))
}

func ensureMongoDBInitFiles(files RuntimeFiles, m Manifest) error {
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		dir := filepath.Join(files.Dir, "providers", "mongodb", instance)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create MongoDB provider directory: %w", err)
		}
		script := `const dbName = process.env.MONGO_INITDB_DATABASE;
const appUser = process.env.BASEHARBOR_MONGODB_USER;
const appPassword = process.env.BASEHARBOR_MONGODB_PASSWORD;
if (!dbName || !appUser || !appPassword) {
  throw new Error("BaseHarbor MongoDB application credentials are not available");
}
const appdb = db.getSiblingDB(dbName);
appdb.createUser({user: appUser, pwd: appPassword, roles: [{role: "readWrite", db: dbName}]});
`
		scriptPath := filepath.Join(dir, "init.js")
		if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
			return fmt.Errorf("write MongoDB application-user init script: %w", err)
		}
		if err := os.Chmod(scriptPath, 0o644); err != nil {
			return fmt.Errorf("make MongoDB application-user init script readable: %w", err)
		}
	}
	return nil
}
