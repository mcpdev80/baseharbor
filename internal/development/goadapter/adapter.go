package goadapter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/extension"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

const (
	AdapterID = "development/go"

	pgxVersion       = "v5.11.0"
	redisVersion     = "v9.22.0"
	awsConfigVersion = "v1.33.6"
	awsS3Version     = "v1.113.4"
	otelVersion      = "v1.46.0"
	mongoVersion     = "v2.9.1"
	amqpVersion      = "v1.15.0"
)

type Adapter struct{}

func (Adapter) Descriptor() extension.Metadata {
	return extension.Metadata{
		SchemaVersion: extension.DescriptorVersion,
		ID:            AdapterID,
		Family:        extension.FamilyDevelopment,
		Version:       "0.1.0",
		Compatibility: extension.Compatibility{
			Contracts: []string{
				"exposure.http/v1",
				"database.sql/v1",
				"cache.key-value/v1",
				"database.key-value/v1",
				"database.document/v1",
				"messaging.queue/v1",
				"messaging.pubsub/v1",
				"messaging.stream/v1",
				"object-storage.s3/v1",
				"secrets/v1",
				"telemetry.otlp/v1",
			},
		},
	}
}

func (Adapter) Detect(root string) (development.Detection, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	path := filepath.Join(root, "go.mod")
	info, err := os.Stat(path)
	if err == nil && info.Mode().IsRegular() {
		return development.Detection{Detected: true, Evidence: []string{"go.mod"}}, nil
	}
	if os.IsNotExist(err) {
		return development.Detection{}, nil
	}
	return development.Detection{}, err
}

func (Adapter) Supports(requirement capability.Requirement) bool {
	switch requirement.Kind {
	case capability.ExposureHTTP, capability.SQL, capability.KeyValue, capability.DurableKeyValue,
		capability.DocumentDatabase, capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream,
		capability.ObjectStorageS3, capability.Secrets, capability.TelemetryOTLP:
		return true
	default:
		return false
	}
}

func (Adapter) Plan(contract application.PortableContract, profile development.StackProfile, component development.Component) ([]development.Action, error) {
	var actions []development.Action
	add := func(kind development.ActionKind, cap capability.Kind, name, value string) {
		actions = append(actions, development.Action{Kind: kind, Component: component.ID, Capability: cap, Name: name, Value: value})
	}

	add(development.ActionBuild, "", "go", "1.25")
	add(development.ActionHealth, capability.ExposureHTTP, "health", "/healthz")

	for _, requirement := range contract.Capabilities {
		if !appliesToComponent(profile, requirement.Kind, component.ID) {
			continue
		}
		if !(Adapter{}).Supports(requirement) {
			continue
		}
		switch requirement.Kind {
		case capability.ExposureHTTP:
			add(development.ActionBinding, requirement.Kind, "PORT", "8080")
			add(development.ActionSource, requirement.Kind, "http", requirement.Name)
		case capability.SQL:
			add(development.ActionDependency, requirement.Kind, "github.com/jackc/pgx/v5", pgxVersion)
			add(development.ActionBinding, requirement.Kind, "DATABASE_URL", "")
			add(development.ActionBinding, requirement.Kind, "DATABASE_CA_FILE", "")
		case capability.KeyValue:
			add(development.ActionDependency, requirement.Kind, "github.com/redis/go-redis/v9", redisVersion)
			add(development.ActionBinding, requirement.Kind, "REDIS_URL", "")
			add(development.ActionBinding, requirement.Kind, "REDIS_CA_FILE", "")
		case capability.DurableKeyValue:
			add(development.ActionDependency, requirement.Kind, "github.com/redis/go-redis/v9", redisVersion)
			add(development.ActionBinding, requirement.Kind, "VALKEY_URL", "")
			add(development.ActionBinding, requirement.Kind, "VALKEY_CA_FILE", "")
		case capability.DocumentDatabase:
			add(development.ActionDependency, requirement.Kind, "go.mongodb.org/mongo-driver/v2", mongoVersion)
			add(development.ActionBinding, requirement.Kind, "MONGODB_URL", "")
			add(development.ActionBinding, requirement.Kind, "MONGODB_CA_FILE", "")
		case capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream:
			add(development.ActionDependency, requirement.Kind, "github.com/rabbitmq/amqp091-go", amqpVersion)
			add(development.ActionBinding, requirement.Kind, "AMQP_URL", "")
			add(development.ActionBinding, requirement.Kind, "RABBITMQ_CA_FILE", "")
		case capability.ObjectStorageS3:
			add(development.ActionDependency, requirement.Kind, "github.com/aws/aws-sdk-go-v2/config", awsConfigVersion)
			add(development.ActionDependency, requirement.Kind, "github.com/aws/aws-sdk-go-v2/service/s3", awsS3Version)
			for _, name := range []string{"S3_ENDPOINT", "S3_BUCKET", "AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_CA_BUNDLE"} {
				add(development.ActionBinding, requirement.Kind, name, "")
			}
		case capability.TelemetryOTLP:
			add(development.ActionDependency, requirement.Kind, "go.opentelemetry.io/otel", otelVersion)
			add(development.ActionDependency, requirement.Kind, "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp", otelVersion)
			add(development.ActionDependency, requirement.Kind, "go.opentelemetry.io/otel/sdk", otelVersion)
			for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_CERTIFICATE", "OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", "OTEL_EXPORTER_OTLP_CLIENT_KEY", "OTEL_SERVICE_NAME", "OTEL_RESOURCE_ATTRIBUTES"} {
				add(development.ActionBinding, requirement.Kind, name, "")
			}
		}
	}
	if contract.Secrets.Managed && appliesToComponent(profile, capability.Secrets, component.ID) {
		for _, requirement := range contract.Secrets.Required {
			add(development.ActionBinding, capability.Secrets, requirement.Name, "")
		}
	}
	return uniqueActions(actions), nil
}

func (Adapter) Bootstrap(plan development.DevelopmentPlan, component development.Component) ([]development.GeneratedFile, error) {
	actions := componentActions(plan, component.ID)
	if len(actions) == 0 {
		return nil, fmt.Errorf("development plan has no actions for component %q", component.ID)
	}
	dependencies := map[string]string{}
	bindings := map[string]struct{}{}
	capabilities := map[capability.Kind]bool{}
	for _, action := range actions {
		if action.Capability != "" {
			capabilities[action.Capability] = true
		}
		switch action.Kind {
		case development.ActionDependency:
			dependencies[action.Name] = action.Value
		case development.ActionBinding:
			bindings[action.Name] = struct{}{}
		}
	}

	return []development.GeneratedFile{
		{Path: "go.mod", Content: []byte(renderGoMod(plan.Application, dependencies)), Mode: 0o644},
		{Path: "main.go", Content: []byte(renderMain(capabilities, bindings)), Mode: 0o644},
		{Path: "Dockerfile", Content: []byte(renderDockerfile()), Mode: 0o644},
		{Path: "compose.yaml", Content: []byte(renderCompose()), Mode: 0o644},
		{Path: ".env.example", Content: []byte(renderEnvExample(bindings)), Mode: 0o644},
	}, nil
}

func (Adapter) Validate(root string, contract application.PortableContract, component development.Component) (development.Validation, error) {
	result, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return development.Validation{}, err
	}
	required := map[capability.Kind]bool{}
	for _, requirement := range contract.Capabilities {
		if (Adapter{}).Supports(requirement) {
			required[requirement.Kind] = false
		}
	}
	if len(contract.Secrets.Required) > 0 {
		required[capability.Secrets] = false
	}
	for _, item := range result.Reconciliation {
		kind := capability.Kind(item.Capability)
		if _, tracked := required[kind]; tracked && item.State == repositoryinspect.ReconciliationSatisfied {
			required[kind] = true
		}
	}
	validation := development.Validation{Satisfied: true, Capabilities: required}
	for kind, satisfied := range required {
		if !satisfied {
			validation.Satisfied = false
			validation.Diagnostics = append(validation.Diagnostics, fmt.Sprintf("%s is not satisfied by repository evidence", kind))
		}
	}
	sort.Strings(validation.Diagnostics)
	return validation, nil
}

func appliesToComponent(profile development.StackProfile, kind capability.Kind, componentID string) bool {
	for _, preference := range profile.Capabilities {
		if preference.Capability != kind {
			continue
		}
		if len(preference.Components) == 0 {
			return true
		}
		for _, id := range preference.Components {
			if id == componentID {
				return true
			}
		}
		return false
	}
	return true
}

func uniqueActions(actions []development.Action) []development.Action {
	seen := map[string]struct{}{}
	result := make([]development.Action, 0, len(actions))
	for _, action := range actions {
		key := string(action.Kind) + "|" + string(action.Capability) + "|" + action.Name + "|" + action.Value
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, action)
	}
	return result
}

func componentActions(plan development.DevelopmentPlan, component string) []development.Action {
	var result []development.Action
	for _, action := range plan.Actions {
		if action.Component == component {
			result = append(result, action)
		}
	}
	return result
}

func renderGoMod(applicationName string, dependencies map[string]string) string {
	moduleName := "example.com/" + strings.Trim(strings.ReplaceAll(strings.ToLower(applicationName), "_", "-"), "-")
	var names []string
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\ngo 1.25.0\n", moduleName)
	if len(names) > 0 {
		b.WriteString("\nrequire (\n")
		for _, name := range names {
			fmt.Fprintf(&b, "\t%s %s\n", name, dependencies[name])
		}
		b.WriteString(")\n")
	}
	return b.String()
}

func renderMain(caps map[capability.Kind]bool, bindings map[string]struct{}) string {
	imports := []string{"\"context\"", "\"fmt\"", "\"log\"", "\"net/http\"", "\"os\"", "\"time\""}
	if caps[capability.SQL] {
		imports = append(imports, "\"github.com/jackc/pgx/v5/pgxpool\"")
	}
	if caps[capability.KeyValue] {
		imports = append(imports, "\"crypto/tls\"", "\"crypto/x509\"", "\"github.com/redis/go-redis/v9\"")
	}
	if caps[capability.ObjectStorageS3] {
		imports = append(imports, "awsconfig \"github.com/aws/aws-sdk-go-v2/config\"", "\"github.com/aws/aws-sdk-go-v2/service/s3\"")
	}
	if caps[capability.TelemetryOTLP] {
		imports = append(imports, "\"go.opentelemetry.io/otel\"", "\"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp\"", "sdktrace \"go.opentelemetry.io/otel/sdk/trace\"")
	}
	sort.Strings(imports)

	var b strings.Builder
	b.WriteString("package main\n\nimport (\n")
	for _, item := range imports {
		fmt.Fprintf(&b, "\t%s\n", item)
	}
	b.WriteString(")\n\n")
	b.WriteString("func requiredEnv(name string) string {\n\tvalue := os.Getenv(name)\n\tif value == \"\" {\n\t\tlog.Fatalf(\"required environment variable %s is not set\", name)\n\t}\n\treturn value\n}\n\n")

	if caps[capability.KeyValue] || caps[capability.DurableKeyValue] || caps[capability.DocumentDatabase] || caps[capability.MessagingQueue] || caps[capability.MessagingPubSub] || caps[capability.MessagingStream] {
		b.WriteString("func tlsConfigFromFile(path string) (*tls.Config, error) {\n\tpem, err := os.ReadFile(path)\n\tif err != nil { return nil, err }\n\troots, err := x509.SystemCertPool()\n\tif err != nil { return nil, err }\n\tif !roots.AppendCertsFromPEM(pem) { return nil, fmt.Errorf(\"no certificates found in %s\", path) }\n\treturn &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, nil\n}\n\n")
	}

	b.WriteString("func main() {\n\tstartup, cancel := context.WithTimeout(context.Background(), 15*time.Second)\n\tdefer cancel()\n")
	if caps[capability.SQL] {
		b.WriteString("\tdb, err := pgxpool.New(startup, requiredEnv(\"DATABASE_URL\"))\n\tif err != nil { log.Fatal(err) }\n\tdefer db.Close()\n\tif err := db.Ping(startup); err != nil { log.Fatalf(\"database readiness: %v\", err) }\n")
	}
	if caps[capability.KeyValue] {
		b.WriteString("\tredisOptions, err := redis.ParseURL(requiredEnv(\"REDIS_URL\"))\n\tif err != nil { log.Fatal(err) }\n\tredisOptions.TLSConfig, err = tlsConfigFromFile(requiredEnv(\"REDIS_CA_FILE\"))\n\tif err != nil { log.Fatal(err) }\n\tcache := redis.NewClient(redisOptions)\n\tdefer cache.Close()\n\tif err := cache.Ping(startup).Err(); err != nil { log.Fatalf(\"cache readiness: %v\", err) }\n")
	}
	if caps[capability.DurableKeyValue] {
		b.WriteString("\tvalkeyOptions, err := redis.ParseURL(requiredEnv(\"VALKEY_URL\"))\n\tif err != nil { log.Fatal(err) }\n\tvalkeyOptions.TLSConfig, err = tlsConfigFromFile(requiredEnv(\"VALKEY_CA_FILE\"))\n\tif err != nil { log.Fatal(err) }\n\tdurableKV := redis.NewClient(valkeyOptions)\n\tdefer durableKV.Close()\n\tif err := durableKV.Ping(startup).Err(); err != nil { log.Fatalf(\"durable key-value readiness: %v\", err) }\n")
	}
	if caps[capability.DocumentDatabase] {
		b.WriteString("\tmongoTLS, err := tlsConfigFromFile(requiredEnv(\"MONGODB_CA_FILE\"))\n\tif err != nil { log.Fatal(err) }\n\tmongoClient, err := mongo.Connect(options.Client().ApplyURI(requiredEnv(\"MONGODB_URL\")).SetTLSConfig(mongoTLS))\n\tif err != nil { log.Fatal(err) }\n\tdefer func() { _ = mongoClient.Disconnect(context.Background()) }()\n\tif err := mongoClient.Ping(startup, nil); err != nil { log.Fatalf(\"document database readiness: %v\", err) }\n")
	}
	if caps[capability.MessagingQueue] || caps[capability.MessagingPubSub] || caps[capability.MessagingStream] {
		b.WriteString("\trabbitTLS, err := tlsConfigFromFile(requiredEnv(\"RABBITMQ_CA_FILE\"))\n\tif err != nil { log.Fatal(err) }\n\trabbit, err := amqp.DialConfig(requiredEnv(\"AMQP_URL\"), amqp.Config{TLSClientConfig: rabbitTLS})\n\tif err != nil { log.Fatalf(\"messaging readiness: %v\", err) }\n\tdefer rabbit.Close()\n\tchannel, err := rabbit.Channel()\n\tif err != nil { log.Fatalf(\"messaging channel readiness: %v\", err) }\n\t_ = channel.Close()\n")
	}
	if caps[capability.ObjectStorageS3] {
		b.WriteString("\tawsCfg, err := awsconfig.LoadDefaultConfig(startup)\n\tif err != nil { log.Fatal(err) }\n\tendpoint := requiredEnv(\"S3_ENDPOINT\")\n\ts3Client := s3.NewFromConfig(awsCfg, func(options *s3.Options) { options.BaseEndpoint = &endpoint; options.UsePathStyle = true })\n\tbucket := requiredEnv(\"S3_BUCKET\")\n\tif _, err := s3Client.HeadBucket(startup, &s3.HeadBucketInput{Bucket: &bucket}); err != nil { log.Fatalf(\"object storage readiness: %v\", err) }\n")
	}
	if caps[capability.Secrets] {
		var names []string
		for name := range bindings {
			if name == "PORT" || strings.Contains(name, "URL") || strings.Contains(name, "CA_") || strings.HasPrefix(name, "S3_") || strings.HasPrefix(name, "AWS_") || strings.HasPrefix(name, "OTEL_") {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "\t_ = requiredEnv(%q)\n", name)
		}
	}
	if caps[capability.TelemetryOTLP] {
		b.WriteString("\texporter, err := otlptracehttp.New(startup)\n\tif err != nil { log.Fatal(err) }\n\ttraces := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))\n\tdefer func() { _ = traces.Shutdown(context.Background()) }()\n\totel.SetTracerProvider(traces)\n")
	}
	b.WriteString("\tport := os.Getenv(\"PORT\")\n\tif port == \"\" { port = \"8080\" }\n\tmux := http.NewServeMux()\n\tmux.HandleFunc(\"/healthz\", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK); _, _ = fmt.Fprintln(w, \"ok\") })\n")
	if caps[capability.TelemetryOTLP] {
		b.WriteString("\tmux.HandleFunc(\"/\", func(w http.ResponseWriter, r *http.Request) { _, span := otel.Tracer(\"app\").Start(r.Context(), \"http.request\"); defer span.End(); _, _ = fmt.Fprintln(w, \"BaseHarbor Go application\") })\n")
	} else {
		b.WriteString("\tmux.HandleFunc(\"/\", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprintln(w, \"BaseHarbor Go application\") })\n")
	}
	b.WriteString("\tserver := &http.Server{Addr: \":\" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}\n\tlog.Printf(\"listening on :%s\", port)\n\tlog.Fatal(server.ListenAndServe())\n}\n")
	return b.String()
}

func renderDockerfile() string {
	return `FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN go mod tidy && go mod verify
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/app .

FROM alpine:3.24.2
RUN apk add --no-cache ca-certificates
COPY --from=build --chown=65532:65532 /out/app /app
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/app"]
`
}

func renderCompose() string {
	return `services:
  app:
    build: .
    environment:
      PORT: "8080"
    expose:
      - "8080"
    healthcheck:
      test: ["CMD", "wget", "-q", "-O", "-", "http://127.0.0.1:8080/healthz"]
      interval: 5s
      timeout: 3s
      retries: 12
      start_period: 5s
`
}

func renderEnvExample(bindings map[string]struct{}) string {
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		if name == "PORT" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "%s=\n", name)
	}
	return b.String()
}
