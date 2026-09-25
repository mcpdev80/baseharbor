package main

import (
	"fmt"
	"io"
	"strings"
	"github.com/mcpdev80/baseharbor/internal/application"
)

func printProjectDetection(out io.Writer, d appProjectDetection) {
	fmt.Fprintf(out, "✓ Application name: %s\n", d.Name)
	if d.Compose != "" {
		fmt.Fprintf(out, "✓ Compose file: %s (read-only)\n", d.Compose)
	} else if len(d.ComposeCandidates) > 1 {
		fmt.Fprintf(out, "? Compose file: %d candidates need confirmation\n", len(d.ComposeCandidates))
	} else {
		fmt.Fprintln(out, "- Compose file: not detected")
	}
	if len(d.WorkloadServices) > 0 {
		fmt.Fprintf(out, "✓ Application workload: %s\n", strings.Join(d.WorkloadServices, ", "))
	}
	if len(d.InfrastructureServices) > 0 {
		fmt.Fprintf(out, "✓ Replaceable repository infrastructure: %s\n", strings.Join(d.InfrastructureServices, ", "))
	}
	if len(d.AmbiguousServices) > 0 {
		fmt.Fprintf(out, "? Compose services need classification: %s\n", strings.Join(d.AmbiguousServices, ", "))
	}
	if d.SQL {
		fmt.Fprintf(out, "✓ SQL Database detected (PostgreSQL-compatible evidence: %s)\n", d.SQLSource)
		if len(d.SQLInstances) > 1 {
			fmt.Fprintf(out, "  logical instances proposed: %s\n", strings.Join(d.SQLInstances, ", "))
		}
	} else {
		fmt.Fprintln(out, "- SQL Database not detected")
	}
	if d.Cache {
		fmt.Fprintf(out, "✓ Cache detected (Redis/Valkey-compatible evidence: %s)\n", d.CacheSource)
		if len(d.CacheInstances) > 1 {
			fmt.Fprintf(out, "  logical instances proposed: %s\n", strings.Join(d.CacheInstances, ", "))
		}
	} else {
		fmt.Fprintln(out, "- Cache not detected")
	}
	switch {
	case d.ObjectStorage:
		fmt.Fprintf(out, "✓ Object Storage detected (S3-compatible evidence: %s)\n", d.ObjectStorageSource)
	case d.ObjectStorageSuggested:
		fmt.Fprintf(out, "? Object Storage suggested (S3-compatible evidence: %s)\n", d.ObjectStorageSource)
	default:
		fmt.Fprintln(out, "- Object Storage not detected")
	}
	switch {
	case d.Metrics:
		fmt.Fprintf(out, "✓ Metrics detected at /metrics (%s)\n", d.MetricsSource)
	case d.MetricsSuggested:
		fmt.Fprintf(out, "? Metrics suggested (%s)\n", d.MetricsSource)
	}
	switch {
	case d.OTLP && len(d.OTLPSignals) > 0:
		fmt.Fprintf(out, "✓ Observability detected (OTLP: %s)\n", strings.Join(d.OTLPSignals, ", "))
	case d.OTLP:
		fmt.Fprintln(out, "? Observability detected via OTLP; signal set needs confirmation")
	case d.OTLPSuggested:
		fmt.Fprintln(out, "? Observability via OTLP suggested")
	}
	if d.LogsSuggested {
		fmt.Fprintln(out, "? Application log collection available for the selected workload")
	}
	if d.RuntimeAPI {
		fmt.Fprintln(out, "✓ BaseHarbor Runtime API usage detected")
	}
	for capabilityID, operations := range d.RuntimePermissions {
		fmt.Fprintf(out, "✓ Runtime operations for %s: %s\n", capabilityID, strings.Join(operations, ", "))
	}
	if len(d.SecretCandidates) > 0 {
		fmt.Fprintln(out, "✓ Potential required secret names:")
		for _, name := range d.SecretCandidates {
			fmt.Fprintf(out, "    %s (%s)\n", name, d.SecretSources[name])
		}
	} else {
		fmt.Fprintln(out, "- Required application secrets not detected")
	}
}

func printManagedCredentialSummary(out io.Writer, selected []bool, runtimePermissions bool) {
	var managed []string
	if len(selected) > 0 && selected[0] {
		managed = append(managed, "SQL service credentials")
	}
	if len(selected) > 1 && selected[1] {
		managed = append(managed, "Cache service credentials")
	}
	if len(selected) > 2 && selected[2] {
		managed = append(managed, "Object-storage access credentials")
	}
	if runtimePermissions {
		managed = append(managed, "Runtime identity / mTLS credentials")
	}
	if len(managed) == 0 {
		return
	}
	fmt.Fprintln(out, "\nManaged automatically by BaseHarbor")
	for _, item := range managed {
		fmt.Fprintf(out, "  - %s\n", item)
	}
	fmt.Fprintln(out, "You do not need to create or enter these managed credentials.")
}

func printAdoptionSummary(out io.Writer, m application.Manifest, detected appProjectDetection, policies []guidedSecretPolicy) {
	fmt.Fprintln(out, "\nAdoption summary")
	fmt.Fprintln(out, "\nApplication")
	fmt.Fprintf(out, "  Name          %s\n", m.Name)
	fmt.Fprintf(out, "  Environment   %s\n", m.Environment)

	if m.Workload.Compose != "" || len(m.Workload.Services) > 0 {
		fmt.Fprintln(out, "\nWorkload")
		if m.Workload.Compose != "" {
			fmt.Fprintf(out, "  Compose       %s (repository-owned, read-only)\n", m.Workload.Compose)
		}
		if len(m.Workload.Services) > 0 {
			fmt.Fprintf(out, "  Services      %s\n", strings.Join(m.Workload.Services, ", "))
		}
	}

	if m.Services.SQL || m.Services.Cache || m.Services.ObjectStorage {
		fmt.Fprintln(out, "\nManaged services")
		if m.Services.SQL {
			fmt.Fprintf(out, "  SQL Database  %s; default provider PostgreSQL\n", adoptionOrigin(detected.SQL))
		}
		if m.Services.Cache {
			fmt.Fprintf(out, "  Cache         %s; default provider Valkey/Redis-compatible\n", adoptionOrigin(detected.Cache))
		}
		if m.Services.ObjectStorage {
			fmt.Fprintf(out, "  Object Storage %s; S3-compatible\n", adoptionOrigin(detected.ObjectStorage))
		}
	}

	if application.HasMetricsSources(m) || application.HasOTLPTelemetry(m) || application.HasLogsCollection(m) {
		fmt.Fprintln(out, "\nObservability")
		if application.HasMetricsSources(m) {
			fmt.Fprintln(out, "  Metrics       expose OpenMetrics HTTP (recommended /metrics)")
			fmt.Fprintln(out, "                BaseHarbor collects workload + supported managed-provider metrics")
		}
		if application.HasLogsCollection(m) {
			fmt.Fprintln(out, "  Logs          write application logs to stdout/stderr")
			fmt.Fprintln(out, "                BaseHarbor collects workload + supported managed-provider logs")
		}
		if application.HasOTLPTelemetry(m) {
			fmt.Fprintf(out, "  Traces/OTLP   %s\n", strings.Join(m.Telemetry.OTLP.Signals, ", "))
			fmt.Fprintln(out, "                BaseHarbor injects the OTLP endpoint/trust binding and collects supported managed-provider traces")
		}
		fmt.Fprintln(out, "  Backends      no Prometheus, Loki/Alloy, Tempo or Collector configuration in application code")
	}

	printGuidedSecretSummary(out, policies)

	if len(m.Runtime.Permissions) > 0 {
		fmt.Fprintln(out, "\nRuntime permissions")
		for _, permission := range m.Runtime.Permissions {
			fmt.Fprintf(out, "  %s\n", permission.Capability)
			fmt.Fprintf(out, "    services: %s\n", strings.Join(permission.Services, ", "))
			fmt.Fprintf(out, "    operations: %s\n", strings.Join(permission.Operations, ", "))
		}
	}
}

func adoptionOrigin(detected bool) string {
	if detected {
		return "detected and confirmed"
	}
	return "user confirmed"
}

func printGuidedSecretSummary(out io.Writer, policies []guidedSecretPolicy) {
	if len(policies) == 0 {
		return
	}
	fmt.Fprintln(out, "\nSecret policy")
	for _, policy := range policies {
		requirement := "optional"
		if policy.Required {
			requirement = "required for startup"
		}
		action := "configure later"
		switch policy.Provision {
		case "prompt":
			action = "ask securely during first apply"
		case "generate":
			action = "generate automatically"
		}
		fmt.Fprintf(out, "  %s: %s; %s\n", policy.Name, requirement, action)
	}
}
