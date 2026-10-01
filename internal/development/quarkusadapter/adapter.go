package quarkusadapter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/extension"
)

const AdapterID = "development/quarkus"

type Adapter struct{}

func (Adapter) Descriptor() extension.Metadata {
	return extension.Metadata{SchemaVersion: extension.DescriptorVersion, ID: AdapterID, Family: extension.FamilyDevelopment, Version: "0.1.0",
		Compatibility: extension.Compatibility{Contracts: []string{"exposure.http/v1", "database.sql/v1", "cache.key-value/v1", "object-storage.s3/v1", "secrets/v1", "telemetry.otlp/v1"}},
	}
}

func (Adapter) Detect(root string) (development.Detection, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	for _, name := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err == nil && strings.Contains(strings.ToLower(string(data)), "quarkus") {
			return development.Detection{Detected: true, Evidence: []string{name}}, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return development.Detection{}, err
		}
	}
	return development.Detection{}, nil
}

func (Adapter) Supports(r capability.Requirement) bool {
	switch r.Kind {
	case capability.ExposureHTTP, capability.SQL, capability.KeyValue, capability.DurableKeyValue,
		capability.DocumentDatabase, capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream,
		capability.ObjectStorageS3, capability.Secrets, capability.TelemetryOTLP:
		return true
	default:
		return false
	}
}

func (a Adapter) Plan(contract application.PortableContract, profile development.StackProfile, component development.Component) ([]development.Action, error) {
	var actions []development.Action
	add := func(k development.ActionKind, c capability.Kind, name, value string) {
		actions = append(actions, development.Action{Kind: k, Component: component.ID, Capability: c, Name: name, Value: value})
	}
	add(development.ActionBuild, "", "maven", "3.9")
	add(development.ActionHealth, capability.ExposureHTTP, "health", "/q/health")
	for _, r := range contract.Capabilities {
		if !a.Supports(r) {
			return nil, fmt.Errorf("Quarkus adapter does not support %s", r.Kind)
		}
		switch r.Kind {
		case capability.ExposureHTTP:
			add(development.ActionDependency, r.Kind, "io.quarkus:quarkus-rest", "")
			add(development.ActionDependency, r.Kind, "io.quarkus:quarkus-smallrye-health", "")
			add(development.ActionBinding, r.Kind, "PORT", "8080")
		case capability.SQL:
			add(development.ActionDependency, r.Kind, "io.quarkus:quarkus-jdbc-postgresql", "")
			add(development.ActionBinding, r.Kind, "DATABASE_URL", "")
		case capability.KeyValue:
			add(development.ActionDependency, r.Kind, "io.quarkus:quarkus-redis-client", "")
			add(development.ActionBinding, r.Kind, "REDIS_URL", "")
			add(development.ActionBinding, r.Kind, "REDIS_CA_FILE", "")
		case capability.DurableKeyValue:
			add(development.ActionDependency, r.Kind, "io.quarkus:quarkus-redis-client", "")
			add(development.ActionBinding, r.Kind, "VALKEY_URL", "")
			add(development.ActionBinding, r.Kind, "VALKEY_CA_FILE", "")
		case capability.DocumentDatabase:
			add(development.ActionDependency, r.Kind, "io.quarkus:quarkus-mongodb-client", "")
			add(development.ActionBinding, r.Kind, "MONGODB_URL", "")
			add(development.ActionBinding, r.Kind, "MONGODB_CA_FILE", "")
		case capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream:
			add(development.ActionDependency, r.Kind, "io.quarkus:quarkus-messaging-rabbitmq", "")
			add(development.ActionBinding, r.Kind, "AMQP_URL", "")
			add(development.ActionBinding, r.Kind, "RABBITMQ_CA_FILE", "")
		case capability.ObjectStorageS3:
			add(development.ActionDependency, r.Kind, "software.amazon.awssdk:s3", "2.55.6")
			add(development.ActionBinding, r.Kind, "S3_ENDPOINT", "")
			add(development.ActionBinding, r.Kind, "S3_BUCKET", "")
		case capability.Secrets:
			for _, s := range contract.Secrets.Required {
				add(development.ActionBinding, r.Kind, s.Name, "")
			}
		case capability.TelemetryOTLP:
			add(development.ActionDependency, r.Kind, "io.quarkus:quarkus-opentelemetry", "")
			add(development.ActionBinding, r.Kind, "OTEL_EXPORTER_OTLP_ENDPOINT", "")
		}
	}
	if contract.Secrets.Managed {
		for _, secret := range contract.Secrets.Required {
			add(development.ActionBinding, capability.Secrets, secret.Name, "")
		}
	}
	return actions, nil
}

func (Adapter) Bootstrap(plan development.DevelopmentPlan, component development.Component) ([]development.GeneratedFile, error) {
	deps := map[string]string{}
	bindings := map[string]struct{}{}
	for _, a := range plan.Actions {
		if a.Component != component.ID {
			continue
		}
		if a.Kind == development.ActionDependency {
			deps[a.Name] = a.Value
		}
		if a.Kind == development.ActionBinding {
			bindings[a.Name] = struct{}{}
		}
	}
	return []development.GeneratedFile{
		{Path: "pom.xml", Content: []byte(renderPom(plan.Application, deps)), Mode: 0o644},
		{Path: "src/main/java/dev/baseharbor/AppResource.java", Content: []byte(renderJava(bindings)), Mode: 0o644},
		{Path: "src/main/resources/application.properties", Content: []byte(renderProperties(bindings)), Mode: 0o644},
		{Path: "Dockerfile", Content: []byte(dockerfile()), Mode: 0o644},
		{Path: "compose.yaml", Content: []byte(compose()), Mode: 0o644},
		{Path: ".env.example", Content: []byte(envExample(bindings)), Mode: 0o644},
	}, nil
}

func (a Adapter) Validate(root string, contract application.PortableContract, component development.Component) (development.Validation, error) {
	return development.ValidateRepositoryCapabilities(root, contract, a.Supports)
}

func renderPom(app string, deps map[string]string) string {
	names := make([]string, 0, len(deps))
	for n := range deps {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("<project xmlns=\"http://maven.apache.org/POM/4.0.0\"><modelVersion>4.0.0</modelVersion>")
	fmt.Fprintf(&b, "<groupId>dev.baseharbor</groupId><artifactId>%s</artifactId><version>0.1.0</version>", strings.ReplaceAll(strings.ToLower(app), "_", "-"))
	b.WriteString("<properties><maven.compiler.release>21</maven.compiler.release><quarkus.platform.group-id>io.quarkus.platform</quarkus.platform.group-id><quarkus.platform.artifact-id>quarkus-bom</quarkus.platform.artifact-id><quarkus.platform.version>3.39.5</quarkus.platform.version></properties>")
	b.WriteString("<dependencyManagement><dependencies><dependency><groupId>io.quarkus.platform</groupId><artifactId>quarkus-bom</artifactId><version>3.39.5</version><type>pom</type><scope>import</scope></dependency></dependencies></dependencyManagement><dependencies>")
	for _, n := range names {
		parts := strings.SplitN(n, ":", 2)
		if len(parts) == 2 {
			fmt.Fprintf(&b, "<dependency><groupId>%s</groupId><artifactId>%s</artifactId>", parts[0], parts[1])
			if deps[n] != "" {
				fmt.Fprintf(&b, "<version>%s</version>", deps[n])
			}
			b.WriteString("</dependency>")
		}
	}
	b.WriteString("</dependencies><build><plugins><plugin><groupId>io.quarkus</groupId><artifactId>quarkus-maven-plugin</artifactId><version>3.39.5</version><extensions>true</extensions></plugin></plugins></build></project>\n")
	return b.String()
}

func renderJava(bindings map[string]struct{}) string {
	names := make([]string, 0, len(bindings))
	for n := range bindings {
		if n != "PORT" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("package dev.baseharbor;\n\nimport jakarta.ws.rs.GET;\nimport jakarta.ws.rs.Path;\nimport jakarta.ws.rs.Produces;\nimport jakarta.ws.rs.core.MediaType;\n\n@Path(\"/\")\npublic class AppResource {\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  private static final String %s = System.getenv(%q);\n", javaIdent(n), n)
	}
	b.WriteString("  @GET @Produces(MediaType.TEXT_PLAIN) public String hello() { return \"BaseHarbor Quarkus application\"; }\n}\n")
	return b.String()
}

func javaIdent(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }

func renderProperties(bindings map[string]struct{}) string {
	var b strings.Builder
	b.WriteString("quarkus.http.host=0.0.0.0\nquarkus.http.port=${PORT:8080}\n")
	if _, ok := bindings["DATABASE_URL"]; ok {
		b.WriteString("quarkus.datasource.jdbc.url=${DATABASE_URL}\n")
	}
	if _, ok := bindings["REDIS_URL"]; ok {
		b.WriteString("quarkus.redis.hosts=${REDIS_URL}\n")
	}
	if _, ok := bindings["VALKEY_URL"]; ok {
		b.WriteString("quarkus.redis.durable.hosts=${VALKEY_URL}\n")
	}
	if _, ok := bindings["MONGODB_URL"]; ok {
		b.WriteString("quarkus.mongodb.connection-string=${MONGODB_URL}\n")
	}
	if _, ok := bindings["OTEL_EXPORTER_OTLP_ENDPOINT"]; ok {
		b.WriteString("quarkus.otel.exporter.otlp.endpoint=${OTEL_EXPORTER_OTLP_ENDPOINT}\n")
	}
	return b.String()
}

func dockerfile() string {
	return "FROM maven:3.9-eclipse-temurin-21 AS build\nWORKDIR /src\nCOPY pom.xml ./\nCOPY src ./src\nRUN mvn -B package -DskipTests\nFROM eclipse-temurin:21-jre\nWORKDIR /app\nCOPY --from=build /src/target/quarkus-app/ ./\nUSER 65532:65532\nEXPOSE 8080\nCMD [\"java\",\"-jar\",\"quarkus-run.jar\"]\n"
}
func compose() string { return "services:\n  app:\n    build: .\n    ports:\n      - \"8080:8080\"\n" }
func envExample(bindings map[string]struct{}) string {
	names := make([]string, 0, len(bindings))
	for n := range bindings {
		if n != "PORT" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "%s=\n", n)
	}
	return b.String()
}
