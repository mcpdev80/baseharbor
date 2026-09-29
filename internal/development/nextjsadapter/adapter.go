package nextjsadapter

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

const AdapterID = "development/nextjs"

type Adapter struct{}

func (Adapter) Descriptor() extension.Metadata {
	return extension.Metadata{
		SchemaVersion: extension.DescriptorVersion,
		ID:            AdapterID,
		Family:        extension.FamilyDevelopment,
		Version:       "0.1.0",
		Compatibility: extension.Compatibility{Contracts: []string{
			"exposure.http/v1", "database.sql/v1", "cache.key-value/v1",
			"object-storage.s3/v1", "secrets/v1", "telemetry.otlp/v1",
		}},
	}
}

func (Adapter) Detect(root string) (development.Detection, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if os.IsNotExist(err) {
		return development.Detection{}, nil
	}
	if err != nil {
		return development.Detection{}, err
	}
	detected := strings.Contains(strings.ToLower(string(data)), "\"next\"")
	var evidence []string
	if detected {
		evidence = []string{"package.json"}
	}
	return development.Detection{Detected: detected, Evidence: evidence}, nil
}

func (Adapter) Supports(r capability.Requirement) bool {
	switch r.Kind {
	case capability.ExposureHTTP, capability.SQL, capability.KeyValue,
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
	add(development.ActionBuild, "", "node", "24.21.0")
	add(development.ActionHealth, capability.ExposureHTTP, "health", "/healthz")
	for _, r := range contract.Capabilities {
		if !a.Supports(r) {
			return nil, fmt.Errorf("Next.js adapter does not support %s", r.Kind)
		}
		switch r.Kind {
		case capability.ExposureHTTP:
			add(development.ActionDependency, r.Kind, "next", "16.3.6")
			add(development.ActionDependency, r.Kind, "react", "19.3.0")
			add(development.ActionDependency, r.Kind, "react-dom", "19.3.0")
			add(development.ActionBinding, r.Kind, "PORT", "3000")
		case capability.SQL:
			add(development.ActionDependency, r.Kind, "pg", "8.23.0")
			add(development.ActionBinding, r.Kind, "DATABASE_URL", "")
		case capability.KeyValue:
			add(development.ActionDependency, r.Kind, "redis", "6.2.1")
			add(development.ActionBinding, r.Kind, "REDIS_URL", "")
		case capability.ObjectStorageS3:
			add(development.ActionDependency, r.Kind, "@aws-sdk/client-s3", "3.1142.0")
			add(development.ActionBinding, r.Kind, "S3_ENDPOINT", "")
			add(development.ActionBinding, r.Kind, "S3_BUCKET", "")
		case capability.Secrets:
			for _, s := range contract.Secrets.Required {
				add(development.ActionBinding, r.Kind, s.Name, "")
			}
		case capability.TelemetryOTLP:
			add(development.ActionDependency, r.Kind, "@opentelemetry/api", "1.9.1")
			add(development.ActionDependency, r.Kind, "@opentelemetry/sdk-node", "0.222.0")
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
	var actions []development.Action
	for _, a := range plan.Actions {
		if a.Component == component.ID {
			actions = append(actions, a)
		}
	}
	deps := map[string]string{}
	bindings := map[string]struct{}{}
	for _, a := range actions {
		if a.Kind == development.ActionDependency {
			deps[a.Name] = a.Value
		}
		if a.Kind == development.ActionBinding {
			bindings[a.Name] = struct{}{}
		}
	}
	return []development.GeneratedFile{
		{Path: "package.json", Content: []byte(renderPackage(plan.Application, deps)), Mode: 0o644},
		{Path: "app/page.tsx", Content: []byte(pageSource()), Mode: 0o644},
		{Path: "app/healthz/route.ts", Content: []byte(healthSource()), Mode: 0o644},
		{Path: "lib/capabilities.ts", Content: []byte(capabilitySource(bindings)), Mode: 0o644},
		{Path: "Dockerfile", Content: []byte(dockerfile()), Mode: 0o644},
		{Path: "compose.yaml", Content: []byte(compose()), Mode: 0o644},
		{Path: ".env.example", Content: []byte(envExample(bindings)), Mode: 0o644},
	}, nil
}

func (a Adapter) Validate(root string, contract application.PortableContract, component development.Component) (development.Validation, error) {
	return development.ValidateRepositoryCapabilities(root, contract, a.Supports)
}

func renderPackage(app string, deps map[string]string) string {
	names := make([]string, 0, len(deps))
	for n := range deps {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"name\": %q,\n  \"private\": true,\n  \"scripts\": {\"dev\": \"next dev\", \"build\": \"next build\", \"start\": \"next start\"},\n  \"dependencies\": {\n", app)
	for i, n := range names {
		comma := ","
		if i == len(names)-1 {
			comma = ""
		}
		fmt.Fprintf(&b, "    %q: %q%s\n", n, deps[n], comma)
	}
	b.WriteString("  },\n  \"devDependencies\": {\"typescript\": \"7.0.2\", \"@types/node\": \"24.19.0\", \"@types/react\": \"19.3.0\"}\n}\n")
	return b.String()
}

func pageSource() string {
	return "import '../lib/capabilities';\nexport default function Page() { return <main>BaseHarbor Next.js application</main>; }\n"
}

func healthSource() string {
	return "import { NextResponse } from 'next/server';\nexport async function GET() { return NextResponse.json({status:'ok'}); }\n"
}

func capabilitySource(bindings map[string]struct{}) string {
	names := make([]string, 0, len(bindings))
	for n := range bindings {
		if n != "PORT" {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("const required = (name: string) => { const value = process.env[name]; if (!value) throw new Error('missing ' + name); return value; };\n")
	for _, n := range names {
		fmt.Fprintf(&b, "export const %s = required(%q);\n", strings.ReplaceAll(strings.ToLower(n), "_", ""), n)
	}
	b.WriteString("// database.sql: pg\n// cache.key-value: ioredis\n// object-storage.s3: @aws-sdk/client-s3\n// telemetry.otlp: @opentelemetry/sdk-node\n")
	return b.String()
}

func dockerfile() string {
	return "FROM node:24.21.0-alpine AS build\nWORKDIR /app\nCOPY package.json ./\nRUN npm install\nCOPY . .\nRUN npm run build\nFROM node:24.21.0-alpine\nWORKDIR /app\nCOPY --from=build /app ./\nUSER node\nEXPOSE 3000\nCMD [\"npm\",\"start\"]\n"
}

func compose() string {
	return "services:\n  app:\n    build: .\n    ports:\n      - \"3000:3000\"\n    healthcheck:\n      test: [\"CMD\",\"wget\",\"-q\",\"-O\",\"-\",\"http://127.0.0.1:3000/healthz\"]\n"
}

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
