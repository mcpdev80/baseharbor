package application

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

const loopbackHost = "127.0.0.1"

type RuntimeContract struct {
	Env                 string
	BindingsDir         string
	WorkloadBindingsDir string
	Metadata            string
}

const workloadServiceBindingDirName = "workload-service-bindings"
const workloadServiceBindingRoot = "/run/baseharbor/service-bindings"

type runtimeMetadata struct {
	Version      int                          `json:"version"`
	Application  string                       `json:"application"`
	Environment  string                       `json:"environment"`
	Services     map[string]runtimeServiceRef `json:"services"`
	Capabilities []capability.Binding         `json:"capabilities,omitempty"`
}

type runtimeServiceRef struct {
	Binding string `json:"binding"`
}

func EnsureRuntimeContract(m Manifest, files RuntimeFiles) (RuntimeContract, error) {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return RuntimeContract{}, err
	}

	bindingsDir := filepath.Join(files.Dir, "bindings")
	bindingsAbs, err := filepath.Abs(bindingsDir)
	if err != nil {
		return RuntimeContract{}, fmt.Errorf("resolve application bindings directory: %w", err)
	}
	if err := os.MkdirAll(bindingsDir, 0o700); err != nil {
		return RuntimeContract{}, fmt.Errorf("create application bindings directory: %w", err)
	}
	if err := os.Chmod(bindingsDir, 0o700); err != nil {
		return RuntimeContract{}, fmt.Errorf("secure application bindings directory: %w", err)
	}

	serviceRefs := map[string]runtimeServiceRef{}
	var env strings.Builder
	fmt.Fprintf(&env, "BASEHARBOR_APP_NAME=%s\n", m.Name)
	fmt.Fprintf(&env, "BASEHARBOR_ENVIRONMENT=%s\n", m.Environment)
	fmt.Fprintf(&env, "BASEHARBOR_BINDINGS=%s\n", bindingsAbs)

	postgresInstances := SQLInstanceNames(m)
	preferredPostgres := preferredServiceInstance(postgresInstances)
	for _, instance := range postgresInstances {
		binding, bindingRef, err := ensureInstanceBindingDirs(bindingsDir, bindingsAbs, "postgres", instance, len(postgresInstances))
		if err != nil {
			return RuntimeContract{}, err
		}
		uri, err := postgresConnectionURL(values, instance)
		if err != nil {
			return RuntimeContract{}, err
		}
		certificates, err := backendCertificates(values[postgresTLSCAKey(instance)])
		if err != nil {
			return RuntimeContract{}, err
		}
		entries := map[string]string{
			"type":         "postgresql",
			"host":         loopbackHost,
			"port":         values[postgresRuntimeKey(instance, "HOST_PORT")],
			"database":     values[postgresRuntimeKey(instance, "DB")],
			"username":     values[postgresRuntimeKey(instance, "USER")],
			"password":     values[postgresRuntimeKey(instance, "PASSWORD")],
			"uri":          uri,
			"certificates": certificates,
		}
		if err := writeBinding(binding, entries); err != nil {
			return RuntimeContract{}, err
		}
		if instance == preferredPostgres {
			fmt.Fprintf(&env, "DATABASE_URL=%s\n", uri)
			fmt.Fprintf(&env, "DATABASE_CA_FILE=%s\n", values[postgresTLSCAKey(instance)])
		}
		if instance != defaultServiceInstance {
			fmt.Fprintf(&env, "DATABASE_%s_URL=%s\n", envInstanceToken(instance), uri)
		}
		serviceRefs[serviceReferenceKey("postgres", instance, len(postgresInstances))] = runtimeServiceRef{Binding: bindingRef}
	}

	redisInstances := ValkeyInstanceNames(m)
	preferredRedis := preferredServiceInstance(redisInstances)
	for _, instance := range redisInstances {
		binding, bindingRef, err := ensureInstanceBindingDirs(bindingsDir, bindingsAbs, "valkey", instance, len(redisInstances))
		if err != nil {
			return RuntimeContract{}, err
		}
		uri, err := valkeyConnectionURL(values, instance)
		if err != nil {
			return RuntimeContract{}, err
		}
		certificates, err := backendCertificates(values[valkeyTLSCAKey(instance)])
		if err != nil {
			return RuntimeContract{}, err
		}
		entries := map[string]string{
			"type":         "redis",
			"host":         loopbackHost,
			"port":         values[valkeyRuntimeKey(instance, "HOST_PORT")],
			"password":     values[valkeyRuntimeKey(instance, "PASSWORD")],
			"uri":          uri,
			"certificates": certificates,
		}
		if err := writeBinding(binding, entries); err != nil {
			return RuntimeContract{}, err
		}
		if instance == preferredRedis {
			fmt.Fprintf(&env, "REDIS_URL=%s\n", uri)
			fmt.Fprintf(&env, "VALKEY_URL=%s\n", uri)
			fmt.Fprintf(&env, "REDIS_CA_FILE=%s\n", values[valkeyTLSCAKey(instance)])
			fmt.Fprintf(&env, "VALKEY_CA_FILE=%s\n", values[valkeyTLSCAKey(instance)])
		}
		if instance != defaultServiceInstance {
			token := envInstanceToken(instance)
			fmt.Fprintf(&env, "REDIS_%s_URL=%s\n", token, uri)
			fmt.Fprintf(&env, "VALKEY_%s_URL=%s\n", token, uri)
		}
		serviceRefs[serviceReferenceKey("valkey", instance, len(redisInstances))] = runtimeServiceRef{Binding: bindingRef}
	}

	rabbitInstances := RabbitMQInstanceNames(m)
	preferredRabbit := preferredServiceInstance(rabbitInstances)
	for _, instance := range rabbitInstances {
		binding, bindingRef, err := ensureInstanceBindingDirs(bindingsDir, bindingsAbs, "rabbitmq", instance, len(rabbitInstances))
		if err != nil {
			return RuntimeContract{}, err
		}
		uri, err := rabbitmqConnectionURL(values, instance)
		if err != nil {
			return RuntimeContract{}, err
		}
		certificates, err := backendCertificates(values[rabbitmqTLSCAKey(instance)])
		if err != nil {
			return RuntimeContract{}, err
		}
		entries := map[string]string{
			"type":         "rabbitmq",
			"provider":     "rabbitmq",
			"host":         loopbackHost,
			"port":         values[rabbitmqRuntimeKey(instance, "HOST_PORT")],
			"username":     values[rabbitmqRuntimeKey(instance, "USER")],
			"password":     values[rabbitmqRuntimeKey(instance, "PASSWORD")],
			"uri":          uri,
			"certificates": certificates,
		}
		if err := writeBinding(binding, entries); err != nil {
			return RuntimeContract{}, err
		}
		if instance == preferredRabbit {
			fmt.Fprintf(&env, "AMQP_URL=%s\n", uri)
			fmt.Fprintf(&env, "RABBITMQ_URL=%s\n", uri)
			fmt.Fprintf(&env, "RABBITMQ_CA_FILE=%s\n", values[rabbitmqTLSCAKey(instance)])
		}
		if instance != defaultServiceInstance {
			token := envInstanceToken(instance)
			fmt.Fprintf(&env, "AMQP_%s_URL=%s\n", token, uri)
			fmt.Fprintf(&env, "RABBITMQ_%s_URL=%s\n", token, uri)
		}
		serviceRefs[serviceReferenceKey("rabbitmq", instance, len(rabbitInstances))] = runtimeServiceRef{Binding: bindingRef}
	}

	mongoInstances := DocumentDatabaseInstanceNames(m)
	preferredMongo := preferredServiceInstance(mongoInstances)
	for _, instance := range mongoInstances {
		binding, bindingRef, err := ensureInstanceBindingDirs(bindingsDir, bindingsAbs, "mongodb", instance, len(mongoInstances))
		if err != nil {
			return RuntimeContract{}, err
		}
		uri, err := mongodbConnectionURL(values, instance)
		if err != nil {
			return RuntimeContract{}, err
		}
		certificates, err := backendCertificates(values[mongodbTLSCAKey(instance)])
		if err != nil {
			return RuntimeContract{}, err
		}
		entries := map[string]string{
			"type":         "mongodb",
			"provider":     "mongodb",
			"host":         loopbackHost,
			"port":         values[mongodbRuntimeKey(instance, "HOST_PORT")],
			"database":     values[mongodbRuntimeKey(instance, "DB")],
			"username":     values[mongodbRuntimeKey(instance, "USER")],
			"password":     values[mongodbRuntimeKey(instance, "PASSWORD")],
			"uri":          uri,
			"certificates": certificates,
		}
		if err := writeBinding(binding, entries); err != nil {
			return RuntimeContract{}, err
		}
		if instance == preferredMongo {
			fmt.Fprintf(&env, "MONGODB_URL=%s\n", uri)
			fmt.Fprintf(&env, "MONGO_URL=%s\n", uri)
			fmt.Fprintf(&env, "MONGODB_CA_FILE=%s\n", values[mongodbTLSCAKey(instance)])
		}
		if instance != defaultServiceInstance {
			token := envInstanceToken(instance)
			fmt.Fprintf(&env, "MONGODB_%s_URL=%s\n", token, uri)
		}
		serviceRefs[serviceReferenceKey("mongodb", instance, len(mongoInstances))] = runtimeServiceRef{Binding: bindingRef}
	}

	applicationEnv := filepath.Join(files.Dir, "application.env")
	if err := writeOwnerOnlyFile(applicationEnv, []byte(env.String())); err != nil {
		return RuntimeContract{}, fmt.Errorf("write application environment contract: %w", err)
	}

	capabilityBindings, err := CapabilityBindings(m)
	if err != nil {
		return RuntimeContract{}, fmt.Errorf("resolve application capability bindings: %w", err)
	}

	metadataPath := filepath.Join(bindingsDir, "metadata.json")
	metadata, err := json.MarshalIndent(runtimeMetadata{
		Version:      1,
		Application:  m.Name,
		Environment:  m.Environment,
		Services:     serviceRefs,
		Capabilities: capabilityBindings,
	}, "", "  ")
	if err != nil {
		return RuntimeContract{}, fmt.Errorf("encode application binding metadata: %w", err)
	}
	metadata = append(metadata, '\n')
	if err := writeOwnerOnlyFile(metadataPath, metadata); err != nil {
		return RuntimeContract{}, fmt.Errorf("write application binding metadata: %w", err)
	}

	workloadBindingsDir, err := ensureWorkloadServiceBindingProjection(m, files, values)
	if err != nil {
		return RuntimeContract{}, err
	}

	return RuntimeContract{
		Env:                 applicationEnv,
		BindingsDir:         bindingsDir,
		WorkloadBindingsDir: workloadBindingsDir,
		Metadata:            metadataPath,
	}, nil
}

func WorkloadServiceBindingProjectionDir(files RuntimeFiles) string {
	return filepath.Join(files.Dir, workloadServiceBindingDirName)
}

func workloadServiceBindingProjectionDir(files RuntimeFiles) string {
	return WorkloadServiceBindingProjectionDir(files)
}

func ensureWorkloadServiceBindingProjection(m Manifest, files RuntimeFiles, values map[string]string) (string, error) {
	root := workloadServiceBindingProjectionDir(files)
	if err := os.RemoveAll(root); err != nil {
		return "", fmt.Errorf("reset workload service binding projection: %w", err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create workload service binding projection: %w", err)
	}

	postgres := SQLInstanceNames(m)
	for _, instance := range postgres {
		name := workloadServiceBindingName("postgres", instance, len(postgres))
		database, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "DB"))
		if err != nil {
			return "", err
		}
		username, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "USER"))
		if err != nil {
			return "", err
		}
		password, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "PASSWORD"))
		if err != nil {
			return "", err
		}
		certificates, err := backendCertificates(values[postgresTLSCAKey(instance)])
		if err != nil {
			return "", err
		}
		certificatePath := filepath.ToSlash(filepath.Join(workloadServiceBindingRoot, name, "certificates"))
		query := url.Values{}
		query.Set("sslmode", "verify-ca")
		query.Set("sslrootcert", certificatePath)
		host := strings.TrimSpace(values[postgresContainerHostKey(instance)])
		if host == "" {
			host = postgresAccessService(instance)
		}
		uri := (&url.URL{
			Scheme:   "postgresql",
			User:     url.UserPassword(username, password),
			Host:     net.JoinHostPort(host, "5432"),
			Path:     "/" + database,
			RawQuery: query.Encode(),
		}).String()
		if err := writeWorkloadServiceBinding(filepath.Join(root, name), map[string]string{
			"type":         "postgresql",
			"provider":     "postgresql",
			"host":         host,
			"port":         "5432",
			"database":     database,
			"username":     username,
			"password":     password,
			"uri":          uri,
			"certificates": certificates,
		}); err != nil {
			return "", fmt.Errorf("project workload service binding %s: %w", name, err)
		}
	}

	cache := ValkeyInstanceNames(m)
	for _, instance := range cache {
		name := workloadServiceBindingName("valkey", instance, len(cache))
		password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
		if err != nil {
			return "", err
		}
		certificates, err := backendCertificates(values[valkeyTLSCAKey(instance)])
		if err != nil {
			return "", err
		}
		host := strings.TrimSpace(values[valkeyContainerHostKey(instance)])
		if host == "" {
			host = valkeyAccessService(instance)
		}
		uri := (&url.URL{
			Scheme: "rediss",
			User:   url.UserPassword("default", password),
			Host:   net.JoinHostPort(host, "6379"),
			Path:   "/0",
		}).String()
		if err := writeWorkloadServiceBinding(filepath.Join(root, name), map[string]string{
			"type":         "redis",
			"provider":     "valkey",
			"host":         host,
			"port":         "6379",
			"username":     "default",
			"password":     password,
			"uri":          uri,
			"certificates": certificates,
		}); err != nil {
			return "", fmt.Errorf("project workload service binding %s: %w", name, err)
		}
	}

	rabbit := RabbitMQInstanceNames(m)
	for _, instance := range rabbit {
		name := workloadServiceBindingName("rabbitmq", instance, len(rabbit))
		username, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "USER"))
		if err != nil {
			return "", err
		}
		password, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "PASSWORD"))
		if err != nil {
			return "", err
		}
		certificates, err := backendCertificates(values[rabbitmqTLSCAKey(instance)])
		if err != nil {
			return "", err
		}
		host := strings.TrimSpace(values[rabbitmqContainerHostKey(instance)])
		if host == "" {
			host = rabbitmqAccessService(instance)
		}
		uri := (&url.URL{
			Scheme: "amqps",
			User:   url.UserPassword(username, password),
			Host:   net.JoinHostPort(host, "5672"),
			Path:   "/",
		}).String()
		if err := writeWorkloadServiceBinding(filepath.Join(root, name), map[string]string{
			"type":         "rabbitmq",
			"provider":     "rabbitmq",
			"host":         host,
			"port":         "5672",
			"username":     username,
			"password":     password,
			"uri":          uri,
			"certificates": certificates,
		}); err != nil {
			return "", fmt.Errorf("project workload RabbitMQ service binding %s: %w", name, err)
		}
	}

	mongoInstances := DocumentDatabaseInstanceNames(m)
	for _, instance := range mongoInstances {
		name := workloadServiceBindingName("mongodb", instance, len(mongoInstances))
		database, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "DB"))
		if err != nil {\n\t\t\treturn "", err\n\t\t}
		username, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "USER"))
		if err != nil {\n\t\t\treturn "", err\n\t\t}
		password, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "PASSWORD"))
		if err != nil {\n\t\t\treturn "", err\n\t\t}
		certificates, err := backendCertificates(values[mongodbTLSCAKey(instance)])
		if err != nil {\n\t\t\treturn "", err\n\t\t}
		host := strings.TrimSpace(values[mongodbContainerHostKey(instance)])
		if host == "" {\n\t\t\thost = mongodbAccessService(instance)\n\t\t}
		uri := mongodbConnectionURI(host, "27017", database, username, password)
		if err := writeWorkloadServiceBinding(filepath.Join(root, name), map[string]string{
			"type": "mongodb", "provider": "mongodb", "host": host, "port": "27017",
			"database": database, "username": username, "password": password,
			"uri": uri, "certificates": certificates,
		}); err != nil {
			return "", fmt.Errorf("project workload MongoDB service binding %s: %w", name, err)
		}
	}
	return root, nil
}

func VerifyWorkloadServiceBindings(m Manifest, files RuntimeFiles) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	root := workloadServiceBindingProjectionDir(files)
	postgres := SQLInstanceNames(m)
	for _, instance := range postgres {
		name := workloadServiceBindingName("postgres", instance, len(postgres))
		entries, err := readWorkloadServiceBinding(filepath.Join(root, name))
		if err != nil {
			return fmt.Errorf("verify workload PostgreSQL binding %s: %w", instance, err)
		}
		if entries["type"] != "postgresql" || entries["provider"] != "postgresql" {
			return fmt.Errorf("verify workload PostgreSQL binding %s: invalid type/provider", instance)
		}
		expectedHost := strings.TrimSpace(values[postgresContainerHostKey(instance)])
		if expectedHost == "" {
			expectedHost = postgresAccessService(instance)
		}
		if entries["host"] != expectedHost || entries["port"] != "5432" {
			return fmt.Errorf("verify workload PostgreSQL binding %s: invalid workload endpoint", instance)
		}
		u, err := url.Parse(entries["uri"])
		if err != nil || u.Scheme != "postgresql" || u.Host != net.JoinHostPort(expectedHost, "5432") {
			return fmt.Errorf("verify workload PostgreSQL binding %s: invalid uri", instance)
		}
		wantCA := filepath.ToSlash(filepath.Join(workloadServiceBindingRoot, name, "certificates"))
		if u.Query().Get("sslmode") != "verify-ca" || u.Query().Get("sslrootcert") != wantCA {
			return fmt.Errorf("verify workload PostgreSQL binding %s: uri trust reference is incomplete", instance)
		}
		if strings.TrimSpace(entries["certificates"]) == "" {
			return fmt.Errorf("verify workload PostgreSQL binding %s: certificates entry is empty", instance)
		}
	}

	cache := ValkeyInstanceNames(m)
	for _, instance := range cache {
		name := workloadServiceBindingName("valkey", instance, len(cache))
		entries, err := readWorkloadServiceBinding(filepath.Join(root, name))
		if err != nil {
			return fmt.Errorf("verify workload cache binding %s: %w", instance, err)
		}
		if entries["type"] != "redis" || entries["provider"] != "valkey" {
			return fmt.Errorf("verify workload cache binding %s: invalid type/provider", instance)
		}
		expectedHost := strings.TrimSpace(values[valkeyContainerHostKey(instance)])
		if expectedHost == "" {
			expectedHost = valkeyAccessService(instance)
		}
		if entries["host"] != expectedHost || entries["port"] != "6379" {
			return fmt.Errorf("verify workload cache binding %s: invalid workload endpoint", instance)
		}
		u, err := url.Parse(entries["uri"])
		if err != nil || u.Scheme != "rediss" || u.Host != net.JoinHostPort(expectedHost, "6379") {
			return fmt.Errorf("verify workload cache binding %s: invalid uri", instance)
		}
		if strings.TrimSpace(entries["certificates"]) == "" {
			return fmt.Errorf("verify workload cache binding %s: certificates entry is empty", instance)
		}
	}

	rabbit := RabbitMQInstanceNames(m)
	for _, instance := range rabbit {
		name := workloadServiceBindingName("rabbitmq", instance, len(rabbit))
		entries, err := readWorkloadServiceBinding(filepath.Join(root, name))
		if err != nil {
			return fmt.Errorf("verify workload RabbitMQ binding %s: %w", instance, err)
		}
		if entries["type"] != "rabbitmq" || entries["provider"] != "rabbitmq" {
			return fmt.Errorf("verify workload RabbitMQ binding %s: invalid type/provider", instance)
		}
		expectedHost := strings.TrimSpace(values[rabbitmqContainerHostKey(instance)])
		if expectedHost == "" {
			expectedHost = rabbitmqAccessService(instance)
		}
		if entries["host"] != expectedHost || entries["port"] != "5672" {
			return fmt.Errorf("verify workload RabbitMQ binding %s: invalid workload endpoint", instance)
		}
		u, err := url.Parse(entries["uri"])
		if err != nil || u.Scheme != "amqps" || u.Host != net.JoinHostPort(expectedHost, "5672") {
			return fmt.Errorf("verify workload RabbitMQ binding %s: invalid uri", instance)
		}
		if strings.TrimSpace(entries["certificates"]) == "" {
			return fmt.Errorf("verify workload RabbitMQ binding %s: certificates entry is empty", instance)
		}
	}

	mongoInstances := DocumentDatabaseInstanceNames(m)
	for _, instance := range mongoInstances {
		name := workloadServiceBindingName("mongodb", instance, len(mongoInstances))
		entries, err := readWorkloadServiceBinding(filepath.Join(root, name))
		if err != nil {\n\t\t\treturn fmt.Errorf("verify workload MongoDB binding %s: %w", instance, err)\n\t\t}
		if entries["type"] != "mongodb" || entries["provider"] != "mongodb" {
			return fmt.Errorf("verify workload MongoDB binding %s: invalid type/provider", instance)
		}
		expectedHost := strings.TrimSpace(values[mongodbContainerHostKey(instance)])
		if expectedHost == "" {\n\t\t\texpectedHost = mongodbAccessService(instance)\n\t\t}
		if entries["host"] != expectedHost || entries["port"] != "27017" {
			return fmt.Errorf("verify workload MongoDB binding %s: invalid workload endpoint", instance)
		}
		u, err := url.Parse(entries["uri"])
		if err != nil || u.Scheme != "mongodb" || u.Host != net.JoinHostPort(expectedHost, "27017") || u.Query().Get("tls") != "true" {
			return fmt.Errorf("verify workload MongoDB binding %s: invalid uri", instance)
		}
		if strings.TrimSpace(entries["certificates"]) == "" {
			return fmt.Errorf("verify workload MongoDB binding %s: certificates entry is empty", instance)
		}
	}
	return nil
}

func readWorkloadServiceBinding(dir string) (map[string]string, error) {
	entries := map[string]string{}
	for _, name := range []string{"type", "provider", "host", "port", "uri", "certificates"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		value := strings.TrimSpace(string(data))
		if value == "" {
			return nil, fmt.Errorf("%s is empty", name)
		}
		entries[name] = value
	}
	return entries, nil
}

func workloadServiceBindingName(kind, instance string, count int) string {
	if count == 1 && instance == defaultServiceInstance {
		return kind
	}
	return kind + "." + instance
}

func writeWorkloadServiceBinding(dir string, entries map[string]string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, value := range entries {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("workload service binding %s has an empty value", name)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(value+"\n"), 0o444); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o444); err != nil {
			return err
		}
	}
	return nil
}

func ensureInstanceBindingDirs(bindingsDir, bindingsAbs, kind, instance string, count int) (string, string, error) {
	kindDir := filepath.Join(bindingsDir, kind)
	kindRef := filepath.Join(bindingsAbs, kind)
	binding := kindDir
	bindingRef := kindRef
	if count != 1 || instance != defaultServiceInstance {
		binding = filepath.Join(kindDir, instance)
		bindingRef = filepath.Join(kindRef, instance)
	}
	if err := os.MkdirAll(binding, 0o700); err != nil {
		return "", "", fmt.Errorf("create %s binding for %s: %w", kind, instance, err)
	}
	for _, path := range []string{kindDir, binding} {
		if err := os.Chmod(path, 0o700); err != nil {
			return "", "", fmt.Errorf("secure %s binding for %s: %w", kind, instance, err)
		}
	}
	return binding, bindingRef, nil
}

func serviceReferenceKey(kind, instance string, count int) string {
	if count == 1 && instance == defaultServiceInstance {
		return kind
	}
	return kind + "." + instance
}

func preferredServiceInstance(instances []string) string {
	if len(instances) == 1 {
		return instances[0]
	}
	for _, preferred := range []string{defaultServiceInstance, "primary"} {
		for _, instance := range instances {
			if instance == preferred {
				return instance
			}
		}
	}
	return ""
}

func envInstanceToken(instance string) string {
	return strings.ToUpper(strings.ReplaceAll(instance, "-", "_"))
}

func postgresConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
	database, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "DB"))
	if err != nil {
		return "", err
	}
	username, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "USER"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	ca, err := requireRuntimeValue(values, postgresTLSCAKey(instance))
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("sslmode", "verify-ca")
	query.Set("sslrootcert", ca)
	u := &url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(username, password),
		Host:     net.JoinHostPort(loopbackHost, port),
		Path:     "/" + database,
		RawQuery: query.Encode(),
	}
	return u.String(), nil
}

func valkeyConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	u := &url.URL{
		Scheme: "rediss",
		User:   url.UserPassword("", password),
		Host:   net.JoinHostPort(loopbackHost, port),
		Path:   "/0",
	}
	return u.String(), nil
}

func rabbitmqConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
	username, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "USER"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	if _, err := requireRuntimeValue(values, rabbitmqTLSCAKey(instance)); err != nil {
		return "", err
	}
	u := &url.URL{
		Scheme: "amqps",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(loopbackHost, port),
		Path:   "/",
	}
	return u.String(), nil
}

func mongodbConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "HOST_PORT"))
	if err != nil {\n\t\t\treturn "", err\n\t\t}
	database, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "DB"))
	if err != nil {\n\t\t\treturn "", err\n\t\t}
	username, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "USER"))
	if err != nil {\n\t\t\treturn "", err\n\t\t}
	password, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "PASSWORD"))
	if err != nil {\n\t\t\treturn "", err\n\t\t}
	if _, err := requireRuntimeValue(values, mongodbTLSCAKey(instance)); err != nil {\n\t\treturn "", err\n\t}
	return mongodbConnectionURI(loopbackHost, port, database, username, password), nil
}


func writeBinding(dir string, values map[string]string) error {
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("application binding %s has an empty value", name)
		}
		if err := writeOwnerOnlyFile(filepath.Join(dir, name), []byte(value+"\n")); err != nil {
			return fmt.Errorf("write application binding %s: %w", name, err)
		}
	}
	return nil
}

func writeOwnerOnlyFile(path string, content []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func readRuntimeEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read application runtime environment: %w", err)
	}
	return readRuntimeEnvBytes(data)
}

func readRuntimeEnvBytes(data []byte) (map[string]string, error) {
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("application runtime environment contains an invalid entry")
		}
		values[key] = value
	}
	return values, nil
}

func requireRuntimeValue(values map[string]string, key string) (string, error) {
	value := strings.TrimSpace(values[key])
	if value == "" {
		return "", fmt.Errorf("application runtime environment is missing %s", key)
	}
	return value, nil
}

func allocateLoopbackPort(exclude map[int]struct{}) (int, error) {
	for attempt := 0; attempt < 16; attempt++ {
		listener, err := net.Listen("tcp", loopbackHost+":0")
		if err != nil {
			return 0, fmt.Errorf("allocate local application service port: %w", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			return 0, fmt.Errorf("release local application service port probe: %w", err)
		}
		if _, exists := exclude[port]; exists {
			continue
		}
		return port, nil
	}
	return 0, fmt.Errorf("allocate distinct local application service port")
}

func validatePortValue(value, key string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("application runtime environment contains invalid %s", key)
	}
	return nil
}
