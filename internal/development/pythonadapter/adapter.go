package pythonadapter

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

const AdapterID = "development/python"

type Adapter struct{}

func (Adapter) Descriptor() extension.Metadata {
	return extension.Metadata{
		SchemaVersion: extension.DescriptorVersion,
		ID:            AdapterID, Family: extension.FamilyDevelopment, Version: "0.1.0",
		Compatibility: extension.Compatibility{Contracts: []string{
			"exposure.http/v1", "database.sql/v1", "cache.key-value/v1",
			"object-storage.s3/v1", "secrets/v1", "telemetry.otlp/v1",
		}},
	}
}

func (Adapter) Detect(root string) (development.Detection, error) {
	if root == "" {
		root = "."
	}
	for _, name := range []string{"pyproject.toml", "requirements.txt"} {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.Mode().IsRegular() {
			return development.Detection{Detected: true, Evidence: []string{name}}, nil
		}
	}
	return development.Detection{}, nil
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
	var out []development.Action
	add := func(k development.ActionKind, c capability.Kind, n, v string) {
		out = append(out, development.Action{Kind: k, Component: component.ID, Capability: c, Name: n, Value: v})
	}
	add(development.ActionBuild, "", "python", "3.14.7")
	add(development.ActionHealth, capability.ExposureHTTP, "health", "/healthz")
	for _, r := range contract.Capabilities {
		if !a.Supports(r) {
			return nil, fmt.Errorf("Python adapter does not support %q", r.Kind)
		}
		switch r.Kind {
		case capability.ExposureHTTP:
			add(development.ActionBinding, r.Kind, "PORT", "8080")
			add(development.ActionSource, r.Kind, "http.server", r.Name)
		case capability.SQL:
			add(development.ActionDependency, r.Kind, "psycopg[binary]", "3.3.6")
			add(development.ActionBinding, r.Kind, "DATABASE_URL", "")
		case capability.KeyValue:
			add(development.ActionDependency, r.Kind, "redis", "8.1.0")
			add(development.ActionBinding, r.Kind, "REDIS_URL", "")
		case capability.ObjectStorageS3:
			add(development.ActionDependency, r.Kind, "boto3", "1.43.104")
			for _, n := range []string{"S3_ENDPOINT", "S3_BUCKET", "AWS_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
				add(development.ActionBinding, r.Kind, n, "")
			}
		case capability.TelemetryOTLP:
			add(development.ActionDependency, r.Kind, "opentelemetry-sdk", "1.45.0")
			add(development.ActionDependency, r.Kind, "opentelemetry-exporter-otlp-proto-http", "1.45.0")
			add(development.ActionBinding, r.Kind, "OTEL_EXPORTER_OTLP_ENDPOINT", "")
		}
	}
	if contract.Secrets.Managed {
		for _, s := range contract.Secrets.Required {
			add(development.ActionBinding, capability.Secrets, s.Name, "")
		}
	}
	return out, nil
}

func (Adapter) Bootstrap(plan development.DevelopmentPlan, component development.Component) ([]development.GeneratedFile, error) {
	var deps, bindings []string
	caps := map[capability.Kind]bool{}
	for _, a := range plan.Actions {
		if a.Component != component.ID {
			continue
		}
		if a.Capability != "" {
			caps[a.Capability] = true
		}
		if a.Kind == development.ActionDependency {
			deps = append(deps, a.Name+"=="+a.Value)
		}
		if a.Kind == development.ActionBinding {
			bindings = append(bindings, a.Name)
		}
	}
	sort.Strings(deps)
	sort.Strings(bindings)
	var pyproject strings.Builder
	pyproject.WriteString("[project]\nname = \"" + plan.Application + "\"\nversion = \"0.1.0\"\nrequires-python = \">=3.14\"\ndependencies = [\n")
	for _, d := range deps {
		fmt.Fprintf(&pyproject, "  %q,\n", d)
	}
	pyproject.WriteString("]\n")
	source := renderPython(caps, bindings)
	env := ""
	for _, b := range bindings {
		if b != "PORT" {
			env += b + "=\n"
		}
	}
	return []development.GeneratedFile{
		{Path: "pyproject.toml", Content: []byte(pyproject.String()), Mode: 0o644},
		{Path: "app.py", Content: []byte(source), Mode: 0o644},
		{Path: "Dockerfile", Content: []byte("FROM python:3.14.7-slim\nWORKDIR /app\nCOPY . .\nRUN pip install --no-cache-dir .\nUSER 65532:65532\nEXPOSE 8080\nCMD [\"python\", \"app.py\"]\n"), Mode: 0o644},
		{Path: "compose.yaml", Content: []byte("services:\n  app:\n    build: .\n    expose: [\"8080\"]\n"), Mode: 0o644},
		{Path: ".env.example", Content: []byte(env), Mode: 0o644},
	}, nil
}

func renderPython(caps map[capability.Kind]bool, bindings []string) string {
	var imports []string
	if caps[capability.SQL] {
		imports = append(imports, "import psycopg")
	}
	if caps[capability.KeyValue] {
		imports = append(imports, "import redis")
	}
	if caps[capability.ObjectStorageS3] {
		imports = append(imports, "import boto3")
	}
	if caps[capability.TelemetryOTLP] {
		imports = append(imports, "from opentelemetry import trace")
	}
	sort.Strings(imports)
	return "import os\nfrom http.server import BaseHTTPRequestHandler, HTTPServer\n" + strings.Join(imports, "\n") + "\n\nAPP_SECRET = os.getenv(\"APP_SECRET\")\n\nclass Handler(BaseHTTPRequestHandler):\n    def do_GET(self):\n        if self.path == \"/healthz\":\n            self.send_response(200); self.end_headers(); self.wfile.write(b\"ok\"); return\n        self.send_response(200); self.end_headers(); self.wfile.write(b\"BaseHarbor Python application\")\n\nHTTPServer((\"0.0.0.0\", int(os.getenv(\"PORT\", \"8080\"))), Handler).serve_forever()\n"
}

func (Adapter) Validate(root string, contract application.PortableContract, component development.Component) (development.Validation, error) {
	r, err := repositoryinspect.Inspect(context.Background(), root)
	if err != nil {
		return development.Validation{}, err
	}
	required := map[capability.Kind]bool{}
	for _, x := range contract.Capabilities {
		if (Adapter{}).Supports(x) {
			required[x.Kind] = false
		}
	}
	for _, item := range r.Reconciliation {
		k := capability.Kind(item.Capability)
		if _, ok := required[k]; ok && item.State == repositoryinspect.ReconciliationSatisfied {
			required[k] = true
		}
	}
	v := development.Validation{Satisfied: true, Capabilities: required}
	for k, ok := range required {
		if !ok {
			v.Satisfied = false
			v.Diagnostics = append(v.Diagnostics, string(k)+" is not satisfied")
		}
	}
	sort.Strings(v.Diagnostics)
	return v, nil
}
