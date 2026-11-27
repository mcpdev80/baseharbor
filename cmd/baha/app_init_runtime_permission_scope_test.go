package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// TestQuickRuntimePermissionScopesToObservedConsumer asserts least privilege at
// the detection→grant boundary: when object storage is detected and exactly ONE
// workload service actually references the S3 runtime API, the inferred
// runtime.create grant must be scoped to that observed service only — never
// spread across all workload services by default.
//
// Candidate (green): the single observed consumer keeps runtime.create and no
// unrelated service inherits it.
func TestQuickRuntimePermissionScopesToObservedConsumer(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
    environment:
      - AWS_S3_BUCKET=tokens
`)
	// A source that creates S3 buckets at runtime lives in the api module only.
	mustWriteWizardTestFile(t, filepath.Join(root, "api.go"), `package main
func provision(client *S3Client) { client.CreateBucket("tokens") }
`)
	withWizardTestDir(t, root)

	d, err := detectAppProject(root)
	if err != nil {
		t.Fatalf("detectAppProject: %v", err)
	}

	if _, ok := d.RuntimePermissions["object-storage.s3/v1"]; !ok {
		t.Fatalf("expected runtime.create inference for object-storage.s3/v1: %#v", d.RuntimePermissions)
	}

	m, err := manifestFromDetectedProject(d, true)
	if err != nil {
		t.Fatalf("manifestFromDetectedProject: %v", err)
	}

	if !application.HasRuntimeCapabilityPermission(m, "object-storage.s3/v1") {
		t.Fatalf("expected object-storage.s3/v1 runtime grant")
	}

	// Least privilege: with a single observed workload, runtime.create must be
	// granted to that observed service only.
	var grantedServices []string
	for _, p := range m.Runtime.Permissions {
		if p.Capability != "object-storage.s3/v1" {
			continue
		}
		for _, op := range p.Operations {
			if op == "runtime.create" {
				grantedServices = append(grantedServices, p.Services...)
			}
		}
	}
	if strings.Join(grantedServices, ",") != "api" {
		t.Fatalf("runtime.create must be scoped to the single observed service [api], got %v:\n%s", grantedServices, marshalManifest(m))
	}

	// The observed consumer (api) keeps runtime.create.
	if !application.RuntimePermissionFor(m, "object-storage.s3/v1", "runtime.create") {
		t.Fatalf("runtime.create missing for observed consumer:\n%s", marshalManifest(m))
	}
}

// TestGuidedRuntimePermissionDoesNotGrantAllWorkloads asserts the same
// least-privilege bound on the interactive guided path: with multiple workload
// services and runtime.create detected, the grant must NOT default to ALL of
// them — the guided path fails closed and asks for the authorized consumer
// instead of broadcasting runtime.create.
func TestGuidedRuntimePermissionDoesNotGrantAllWorkloads(t *testing.T) {
	root := t.TempDir()
	mustWriteWizardTestFile(t, filepath.Join(root, "compose.yaml"), `services:
  api:
    image: example/api
    environment:
      - AWS_S3_BUCKET=tokens
  worker:
    image: example/worker
  db:
    image: example/db
`)
	mustWriteWizardTestFile(t, filepath.Join(root, "api.go"), `package main
func provision(client *S3Client) { client.CreateBucket("tokens") }
`)
	withWizardTestDir(t, root)

	d, err := detectAppProject(root)
	if err != nil {
		t.Fatalf("detectAppProject: %v", err)
	}
	if _, ok := d.RuntimePermissions["object-storage.s3/v1"]; !ok {
		t.Fatalf("expected runtime.create inference for object-storage.s3/v1")
	}

	// Drive the interactive guided selection with a shared reader (mirrors
	// runAppInitWizard) that confirms every default prompt.
	reader := bufio.NewReader(strings.NewReader(strings.Repeat("\n", 80)))
	selection, err := collectGuidedInitSelection(reader, &noopWriter{}, d)
	if err != nil {
		t.Fatalf("collectGuidedInitSelection: %v", err)
	}
	m, err := buildGuidedInitManifest(reader, &noopWriter{}, d, selection)
	if err == nil {
		t.Fatalf("runtime.create must NOT be granted when multiple workload services observed the runtime API:\n%s", marshalManifest(m))
	}
	if !strings.Contains(err.Error(), "runtime.create was detected but more than one workload service is selected") {
		t.Fatalf("expected usage error when runtime.create scope is ambiguous across multiple workloads, got: %v", err)
	}

	// Negative guard: even if some manifest were produced, no non-consuming
	// service may hold runtime.create.
	if m.Runtime.Permissions != nil {
		for _, p := range m.Runtime.Permissions {
			if p.Capability != "object-storage.s3/v1" {
				continue
			}
			for _, svc := range p.Services {
				if svc == "worker" || svc == "db" {
					t.Fatalf("runtime.create leaked to non-consuming service %q:\n%s", svc, marshalManifest(m))
				}
			}
		}
	}
}

// TestRuntimePermissionRequiresAttribution pins the attribution selector.
func TestRuntimePermissionRequiresAttribution(t *testing.T) {
	cases := []struct {
		name   string
		ops    []string
		attrib bool
	}{
		{"none", nil, false},
		{"runtime.get only", []string{"runtime.get", "runtime.delete"}, false},
		{"runtime.create present", []string{"runtime.create", "runtime.get"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runtimePermissionRequiresAttribution(tc.ops); got != tc.attrib {
				t.Fatalf("runtimePermissionRequiresAttribution(%v) = %t, want %t", tc.ops, got, tc.attrib)
			}
		})
	}
}

func marshalManifest(m application.Manifest) string {
	return m.YAML()
}

// noopWriter discards output for the guided-manifest probe.
type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }

// usageErrType is a structural type-check helper: usageError returns a
// *cli.UsageError, so we compare via a sentinel error rather than importing the
// concrete type. It returns the sentinel carried by usageError.
func usageErrType() error {
	return usageError("", "")
}

// ensure os import is retained for potential fixture use
var _ = os.Stdout
