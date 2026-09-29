package development

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/extension"
	"github.com/mcpdev80/baseharbor/internal/repositoryinspect"
)

const GoAdapterID = "baseharbor/go"

type GoAdapter struct{}

func (GoAdapter) Descriptor() extension.Metadata {
	return extension.Metadata{
		SchemaVersion: extension.DescriptorVersion,
		ID:            GoAdapterID,
		Family:        extension.FamilyDevelopment,
		Version:       "0.1.0",
		Compatibility: extension.Compatibility{
			Contracts: []string{
				"database.sql/v1",
				"cache.key-value/v1",
				"object-storage.s3/v1",
				"secrets/v1",
				"exposure.http/v1",
				"telemetry.otlp/v1",
			},
		},
	}
}

func (GoAdapter) Detect(root string) (Detection, error) {
	result, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return Detection{}, err
	}
	var evidence []string
	for _, artifact := range result.Artifacts {
		if artifact.Path == "go.mod" {
			evidence = append(evidence, artifact.Path)
		}
	}
	return Detection{Detected: len(evidence) > 0, Evidence: evidence}, nil
}

func (GoAdapter) Supports(requirement capability.Requirement) bool {
	switch requirement.Kind {
	case capability.SQL, capability.KeyValue, capability.ObjectStorageS3,
		capability.Secrets, capability.ExposureHTTP, capability.TelemetryOTLP:
		return true
	default:
		return false
	}
}

func (a GoAdapter) Plan(contract application.PortableContract, profile StackProfile, component Component) ([]Action, error) {
	var actions []Action
	for _, requirement := range contract.Capabilities {
		if !a.Supports(requirement) {
			return nil, fmt.Errorf("Go adapter does not support capability %q", requirement.Kind)
		}
		switch requirement.Kind {
		case capability.SQL:
			actions = append(actions, Action{Kind: ActionDependency, Capability: requirement.Kind, Name: "github.com/jackc/pgx/v5", Value: "v5.11.0"})
		case capability.KeyValue:
			actions = append(actions, Action{Kind: ActionDependency, Capability: requirement.Kind, Name: "github.com/redis/go-redis/v9", Value: "v9.22.0"})
		case capability.ObjectStorageS3:
			actions = append(actions,
				Action{Kind: ActionDependency, Capability: requirement.Kind, Name: "github.com/aws/aws-sdk-go-v2/config", Value: "v1.33.6"},
				Action{Kind: ActionDependency, Capability: requirement.Kind, Name: "github.com/aws/aws-sdk-go-v2/service/s3", Value: "v1.113.4"},
			)
		case capability.TelemetryOTLP:
			actions = append(actions, Action{Kind: ActionDependency, Capability: requirement.Kind, Name: "go.opentelemetry.io/otel", Value: "v1.46.0"})
		}
	}
	actions = append(actions,
		Action{Kind: ActionSource, Name: "cmd/app/main.go"},
		Action{Kind: ActionBuild, Name: "Dockerfile"},
		Action{Kind: ActionHealth, Name: "/healthz"},
	)
	return actions, nil
}

func (GoAdapter) Bootstrap(plan DevelopmentPlan, component Component) ([]GeneratedFile, error) {
	module := sanitizeModule(plan.Application)
	return []GeneratedFile{
		{Path: "go.mod", Mode: 0o644, Content: []byte(goModule(module, plan.Actions))},
		{Path: "cmd/app/main.go", Mode: 0o644, Content: []byte(goMainSource(plan.Application))},
		{Path: "Dockerfile", Mode: 0o644, Content: []byte(goDockerfile())},
		{Path: "compose.yaml", Mode: 0o644, Content: []byte(goCompose())},
		{Path: ".env.example", Mode: 0o644, Content: []byte(goEnvExample())},
	}, nil
}

func (GoAdapter) Validate(root string, contract application.PortableContract, component Component) (Validation, error) {
	result, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return Validation{}, err
	}
	validation := Validation{Satisfied: true, Capabilities: map[capability.Kind]bool{}}
	for _, requirement := range contract.Capabilities {
		satisfied := false
		for _, finding := range result.Findings {
			if finding.Capability == string(requirement.Kind) && finding.Confidence == repositoryinspect.ConfidenceDetected {
				satisfied = true
				break
			}
		}
		validation.Capabilities[requirement.Kind] = satisfied
		if !satisfied {
			validation.Satisfied = false
			validation.Diagnostics = append(validation.Diagnostics, "missing evidence for "+string(requirement.Kind))
		}
	}
	return validation, nil
}

func sanitizeModule(app string) string {
	app = strings.ToLower(strings.TrimSpace(app))
	app = strings.ReplaceAll(app, "_", "-")
	app = strings.ReplaceAll(app, " ", "-")
	if app == "" {
		app = "app"
	}
	return "example.com/" + app
}

func goModule(module string, actions []Action) string {
	deps := map[string]string{}
	for _, action := range actions {
		if action.Kind == ActionDependency {
			deps[action.Name] = action.Value
		}
	}
	names := make([]string, 0, len(deps))
	for name := range deps {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\ngo 1.25.0\n", module)
	if len(names) > 0 {
		b.WriteString("\nrequire (\n")
		for _, name := range names {
			fmt.Fprintf(&b, "\t%s %s\n", name, deps[name])
		}
		b.WriteString(")\n")
	}
	return b.String()
}

func goMainSource(app string) string {
	return `package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		response, err := client.Get("http://127.0.0.1:8080/healthz")
		if err != nil || response.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		_ = response.Body.Close()
		return
	}

	ctx := context.Background()
	if raw := os.Getenv("DATABASE_URL"); raw != "" {
		if pool, err := pgxpool.New(ctx, raw); err == nil {
			defer pool.Close()
		}
	}
	if raw := os.Getenv("REDIS_URL"); raw != "" {
		if options, err := redis.ParseURL(raw); err == nil {
			client := redis.NewClient(options)
			defer client.Close()
		}
	}
	if cfg, err := config.LoadDefaultConfig(ctx); err == nil {
		_ = s3.NewFromConfig(cfg)
	}
	_ = otel.Tracer("` + app + `")

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	log.Fatal(http.ListenAndServe(":8080", mux))
}
`
}

func goDockerfile() string {
	return "FROM golang:1.25 AS build\nWORKDIR /src\nCOPY go.mod go.sum* ./\nRUN go mod download\nCOPY . .\nRUN CGO_ENABLED=0 go build -o /out/app ./cmd/app\nFROM gcr.io/distroless/static-debian12:nonroot\nCOPY --from=build /out/app /app\nUSER nonroot:nonroot\nENTRYPOINT [\"/app\"]\n"
}

func goCompose() string {
	return "services:\n  app:\n    build: .\n    ports:\n      - \"8080:8080\"\n    labels:\n      io.baseharbor.workload.protocol: http\n    environment:\n      DATABASE_URL: ${DATABASE_URL}\n      REDIS_URL: ${REDIS_URL}\n      S3_ENDPOINT: ${S3_ENDPOINT}\n      APP_SECRET: ${APP_SECRET}\n      OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT}\n    healthcheck:\n      test: [\"CMD\", \"/app\", \"--healthcheck\"]\n"
}

func goEnvExample() string {
	return "DATABASE_URL=\nREDIS_URL=\nS3_ENDPOINT=\nAPP_SECRET=\nOTEL_EXPORTER_OTLP_ENDPOINT=\n"
}
