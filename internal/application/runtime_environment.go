package application

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ensureRuntimeEnv(path string, m Manifest) error {
	values := map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		var readErr error
		values, readErr = readRuntimeEnvBytes(data)
		if readErr != nil {
			return readErr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect application runtime environment: %w", err)
	}

	if err := validateProtectedRuntimeValues(values); err != nil {
		return err
	}
	if err := validateRetainedRuntimeCredentials(path, values, m); err != nil {
		return err
	}
	if err := ensureDesiredRuntimeValues(values, m); err != nil {
		return err
	}
	if err := ensureNativeBackendCredentialValues(values, m); err != nil {
		return err
	}
	if err := ensureNativeBackendEndpointValues(values, m); err != nil {
		return err
	}
	if err := validateRuntimeValues(values, m); err != nil {
		return err
	}
	return writeRuntimeEnv(path, m, values)
}

func newRuntimeEnv(m Manifest) (string, error) {
	values := map[string]string{}
	if err := ensureDesiredRuntimeValues(values, m); err != nil {
		return "", err
	}
	if err := ensureNativeBackendEndpointValues(values, m); err != nil {
		return "", err
	}
	return runtimeEnvContent(m, values), nil
}

func ensureDesiredRuntimeValues(values map[string]string, m Manifest) error {
	excluded := map[int]struct{}{}
	for key, value := range values {
		if !strings.HasSuffix(key, "_HOST_PORT") || strings.TrimSpace(value) == "" {
			continue
		}
		if err := validatePortValue(value, key); err != nil {
			return errors.New("protected application runtime environment contains an invalid port reference")
		}
		port, _ := strconv.Atoi(value)
		excluded[port] = struct{}{}
	}

	for _, instance := range SQLInstanceNames(m) {
		dbKey := postgresRuntimeKey(instance, "DB")
		userKey := postgresRuntimeKey(instance, "USER")
		passwordKey := postgresRuntimeKey(instance, "PASSWORD")
		portKey := postgresRuntimeKey(instance, "HOST_PORT")
		if values[dbKey] == "" {
			values[dbKey] = postgresDatabaseName(m, instance)
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
	for _, instance := range ValkeyInstanceNames(m) {
		passwordKey := valkeyRuntimeKey(instance, "PASSWORD")
		portKey := valkeyRuntimeKey(instance, "HOST_PORT")
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
	for _, instance := range RabbitMQInstanceNames(m) {
		userKey := rabbitmqRuntimeKey(instance, "USER")
		passwordKey := rabbitmqRuntimeKey(instance, "PASSWORD")
		portKey := rabbitmqRuntimeKey(instance, "HOST_PORT")
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
		bootstrapUserKey := rabbitmqRuntimeKey(instance, "BOOTSTRAP_USER")
		bootstrapPasswordKey := rabbitmqRuntimeKey(instance, "BOOTSTRAP_PASSWORD")
		if values[bootstrapUserKey] == "" {
			values[bootstrapUserKey] = "baseharbor_internal"
		}
		if values[bootstrapPasswordKey] == "" {
			password, err := randomApplicationSecret(32)
			if err != nil {
				return err
			}
			values[bootstrapPasswordKey] = password
		}
		if m.Services.MessagingManagementUI {
			adminUserKey := rabbitmqRuntimeKey(instance, "ADMIN_USER")
			adminPasswordKey := rabbitmqRuntimeKey(instance, "ADMIN_PASSWORD")
			if values[adminUserKey] == "" {
				values[adminUserKey] = "developer"
			}
			if values[adminPasswordKey] == "" {
				password, err := randomApplicationSecret(32)
				if err != nil {
					return err
				}
				values[adminPasswordKey] = password
			}
		}
		if rabbitmqMemberCount(m) > 1 {
			cookieKey := rabbitmqRuntimeKey(instance, "ERLANG_COOKIE")
			if values[cookieKey] == "" {
				cookie, err := randomApplicationSecret(32)
				if err != nil {
					return err
				}
				values[cookieKey] = cookie
			}
		}
		if values[portKey] == "" {
			port, err := allocateLoopbackPort(excluded)
			if err != nil {
				return err
			}
			values[portKey] = strconv.Itoa(port)
			excluded[port] = struct{}{}
		}
		if m.Services.MessagingManagementUI {
			uiPortKey := rabbitmqUIHostPortKey(instance)
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
	if err := ensureMongoDBRuntimeValues(values, m, excluded); err != nil {
		return err
	}
	if m.Services.SQLManagementUI {
		if values[PostgresUIEmailEnv] == "" {
			values[PostgresUIEmailEnv] = "baseharbor@example.com"
		}
		if values[PostgresUIPasswordEnv] == "" {
			value, err := randomApplicationSecret(24)
			if err != nil {
				return err
			}
			values[PostgresUIPasswordEnv] = value
		}
		if values[PostgresUIHostPortEnv] == "" {
			port, err := allocateLoopbackPort(excluded)
			if err != nil {
				return err
			}
			values[PostgresUIHostPortEnv] = strconv.Itoa(port)
			excluded[port] = struct{}{}
		}
	}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		if values[CacheUIUserEnv] == "" {
			values[CacheUIUserEnv] = "baseharbor"
		}
		if values[CacheUIPasswordEnv] == "" {
			value, err := randomApplicationSecret(24)
			if err != nil {
				return err
			}
			values[CacheUIPasswordEnv] = value
		}
		if values[CacheUIHostPortEnv] == "" {
			port, err := allocateLoopbackPort(excluded)
			if err != nil {
				return err
			}
			values[CacheUIHostPortEnv] = strconv.Itoa(port)
			excluded[port] = struct{}{}
		}
	}
	for _, bucket := range ObjectStorageBucketNames(m) {
		accessKey := s3RuntimeKey(bucket, "ACCESS_KEY_ID")
		secretKey := s3RuntimeKey(bucket, "SECRET_ACCESS_KEY")
		if values[accessKey] == "" {
			value, err := randomApplicationSecret(18)
			if err != nil {
				return err
			}
			values[accessKey] = "BH" + value
		}
		if values[secretKey] == "" {
			value, err := randomApplicationSecret(32)
			if err != nil {
				return err
			}
			values[secretKey] = value
		}
	}
	return nil
}

func writeRuntimeEnv(path string, m Manifest, values map[string]string) error {
	if err := validateProtectedRuntimeValues(values); err != nil {
		return err
	}
	if err := writeOwnerOnlyFile(path, []byte(runtimeEnvContent(m, values))); err != nil {
		return fmt.Errorf("write application runtime environment: %w", err)
	}
	return nil
}

func runtimeEnvContent(m Manifest, values map[string]string) string {
	var b strings.Builder
	for _, instance := range SQLInstanceNames(m) {
		for _, suffix := range []string{"DB", "USER", "PASSWORD", "HOST_PORT", "TLS_CA_FILE", "CONTAINER_HOST"} {
			key := postgresRuntimeKey(instance, suffix)
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
	}
	for _, instance := range ValkeyInstanceNames(m) {
		for _, suffix := range []string{"PASSWORD", "HOST_PORT", "TLS_CA_FILE", "CONTAINER_HOST", "USER", "KEY_PREFIX"} {
			key := valkeyRuntimeKey(instance, suffix)
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		suffixes := []string{"USER", "PASSWORD", "BOOTSTRAP_USER", "BOOTSTRAP_PASSWORD", "HOST_PORT", "TLS_CA_FILE", "CONTAINER_HOST"}
		if m.Services.MessagingManagementUI {
			suffixes = append(suffixes, "ADMIN_USER", "ADMIN_PASSWORD")
		}
		if rabbitmqMemberCount(m) > 1 {
			suffixes = append(suffixes, "ERLANG_COOKIE")
		}
		for _, suffix := range suffixes {
			key := rabbitmqRuntimeKey(instance, suffix)
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
		if m.Services.MessagingManagementUI {
			key := rabbitmqUIHostPortKey(instance)
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
	}
	appendMongoDBRuntimeEnv(&b, m, values)
	if m.Services.SQLManagementUI {
		for _, key := range []string{PostgresUIHostPortEnv, PostgresUIEmailEnv, PostgresUIPasswordEnv} {
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
	}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		for _, key := range []string{CacheUIHostPortEnv, CacheUIUserEnv, CacheUIPasswordEnv} {
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
	}
	if values[S3TLSHostCAEnv] != "" {
		fmt.Fprintf(&b, "%s=%s\n", S3TLSHostCAEnv, values[S3TLSHostCAEnv])
	}
	for _, bucket := range ObjectStorageBucketNames(m) {
		for _, suffix := range []string{"ACCESS_KEY_ID", "SECRET_ACCESS_KEY"} {
			key := s3RuntimeKey(bucket, suffix)
			fmt.Fprintf(&b, "%s=%s\n", key, values[key])
		}
		for _, suffix := range []string{"BUCKET", "HOST_ENDPOINT", "CONTAINER_ENDPOINT"} {
			key := s3RuntimeKey(bucket, suffix)
			if values[key] != "" {
				fmt.Fprintf(&b, "%s=%s\n", key, values[key])
			}
		}
	}
	if HasOTLPTelemetry(m) {
		for _, key := range []string{"OTLP_PROVIDER", "OTLP_HOST_ENDPOINT", "OTLP_CONTAINER_ENDPOINT", OTLPTLSHostCAEnv, OTLPTLSHostClientCertEnv, OTLPTLSHostClientKeyEnv} {
			if values[key] != "" {
				fmt.Fprintf(&b, "%s=%s\n", key, values[key])
			}
		}
	}
	if HasIdentity(m) {
		for _, key := range []string{"IDENTITY_CONTAINER_ISSUER", "IDENTITY_CLIENT_ID", "IDENTITY_CLIENT_SECRET", "IDENTITY_CA_FILE"} {
			if values[key] != "" {
				fmt.Fprintf(&b, "%s=%s\n", key, values[key])
			}
		}
	}
	appendRetainedRuntimeValues(&b, values)
	return b.String()
}

func RuntimeEnvironment(files RuntimeFiles) (map[string]string, error) {
	return readRuntimeEnv(files.Env)
}

func validateRuntimeEnv(path string, m Manifest) error {
	values, err := readRuntimeEnv(path)
	if err != nil {
		return err
	}
	return validateRuntimeValues(values, m)
}

func validateRuntimeValues(values map[string]string, m Manifest) error {
	for _, instance := range SQLInstanceNames(m) {
		for _, suffix := range []string{"DB", "USER", "PASSWORD", "HOST_PORT"} {
			key := postgresRuntimeKey(instance, suffix)
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		portKey := postgresRuntimeKey(instance, "HOST_PORT")
		if err := validatePortValue(values[portKey], portKey); err != nil {
			return err
		}
	}
	for _, instance := range ValkeyInstanceNames(m) {
		for _, suffix := range []string{"PASSWORD", "HOST_PORT"} {
			key := valkeyRuntimeKey(instance, suffix)
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		portKey := valkeyRuntimeKey(instance, "HOST_PORT")
		if err := validatePortValue(values[portKey], portKey); err != nil {
			return err
		}
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		for _, suffix := range []string{"USER", "PASSWORD", "HOST_PORT"} {
			key := rabbitmqRuntimeKey(instance, suffix)
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		portKey := rabbitmqRuntimeKey(instance, "HOST_PORT")
		if err := validatePortValue(values[portKey], portKey); err != nil {
			return err
		}
		if m.Services.MessagingManagementUI {
			uiPortKey := rabbitmqUIHostPortKey(instance)
			if values[uiPortKey] == "" {
				return fmt.Errorf("application runtime environment is missing %s", uiPortKey)
			}
			if err := validatePortValue(values[uiPortKey], uiPortKey); err != nil {
				return err
			}
		}
	}
	if err := validateMongoDBRuntimeValues(values, m); err != nil {
		return err
	}
	if m.Services.SQLManagementUI {
		for _, key := range []string{PostgresUIHostPortEnv, PostgresUIEmailEnv, PostgresUIPasswordEnv} {
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		if err := validatePortValue(values[PostgresUIHostPortEnv], PostgresUIHostPortEnv); err != nil {
			return err
		}
	}
	if m.Services.CacheManagementUI || m.Services.KeyValueManagementUI {
		for _, key := range []string{CacheUIHostPortEnv, CacheUIUserEnv, CacheUIPasswordEnv} {
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
		if err := validatePortValue(values[CacheUIHostPortEnv], CacheUIHostPortEnv); err != nil {
			return err
		}
	}
	for _, bucket := range ObjectStorageBucketNames(m) {
		for _, suffix := range []string{"ACCESS_KEY_ID", "SECRET_ACCESS_KEY"} {
			key := s3RuntimeKey(bucket, suffix)
			if values[key] == "" {
				return fmt.Errorf("application runtime environment is missing %s", key)
			}
		}
	}
	return nil
}

func postgresDatabaseName(m Manifest, instance string) string {
	base := strings.ReplaceAll(m.Name+"_"+m.Environment, "-", "_")
	if instance == defaultServiceInstance {
		if len(base) > 63 {
			base = base[:63]
		}
		return base
	}
	suffix := "_" + strings.ReplaceAll(instance, "-", "_")
	maxBase := 63 - len(suffix)
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + suffix
}

func runtimeServiceName(kind, instance string) string {
	if instance == defaultServiceInstance {
		return kind
	}
	return kind + "-" + instance
}

func postgresRuntimeKey(instance, suffix string) string {
	return runtimeInstanceKey("POSTGRES", instance, suffix)
}

func valkeyRuntimeKey(instance, suffix string) string {
	return runtimeInstanceKey("VALKEY", instance, suffix)
}

func rabbitmqRuntimeKey(instance, suffix string) string {
	return runtimeInstanceKey("RABBITMQ", instance, suffix)
}

func rabbitmqContainerHostKey(instance string) string {
	return rabbitmqRuntimeKey(instance, "CONTAINER_HOST")
}

func s3RuntimeKey(bucket, suffix string) string {
	return runtimeInstanceKey("S3", bucket, suffix)
}

func runtimeInstanceKey(prefix, instance, suffix string) string {
	if instance == defaultServiceInstance {
		return prefix + "_" + suffix
	}
	name := strings.ToUpper(strings.ReplaceAll(instance, "-", "_"))
	return prefix + "_" + name + "_" + suffix
}

func randomApplicationSecret(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate application runtime secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
