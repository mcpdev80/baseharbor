package repositoryinspect

import (
	"context"
	"path/filepath"
	"strings"
)

type objectStorageDetector struct{}

func (objectStorageDetector) Name() string { return "object-storage.s3" }

func (objectStorageDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	var detected, suggested []Evidence
	var runtimeCreate []Evidence
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		base := strings.ToLower(filepath.Base(path))
		lower := strings.ToLower(string(data))

		if isEnvFile(base) {
			for _, name := range readEnvNames(data) {
				switch strings.ToUpper(name) {
				case "S3_ENDPOINT", "S3_BUCKET", "AWS_ENDPOINT_URL", "AWS_S3_BUCKET":
					detected = append(detected, Evidence{Kind: EvidenceEnv, Path: path, Detail: "variable " + name})
				}
			}
		}
		if isDependencyFile(base) && containsAnyToken(lower, []string{
			"@aws-sdk/client-s3", "aws-sdk-s3", "boto3", "botocore", "minio",
		}) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceDependency, Path: path,
				Detail: "dependency metadata references an S3-compatible client",
			})
		}
		if isSourceFile(base) {
			if containsAny(lower, []string{
				"putobject(", "getobject(", "put_object(", "get_object(",
				"s3client", "client-s3", "boto3.client(\\\"s3\\\")", "boto3.client('s3')",
			}) {
				suggested = append(suggested, Evidence{
					Kind: EvidenceImport, Path: path,
					Detail: "source references S3-compatible object operations",
				})
			}
			if containsAny(lower, []string{
				"createbucket(", ".createbucket(", "create_bucket(", ".create_bucket(",
			}) {
				runtimeCreate = append(runtimeCreate, Evidence{
					Kind: EvidenceCall, Path: path,
					Detail: "source appears to create S3 buckets at application runtime",
				})
			}
		}
	}

	evidence := append([]Evidence(nil), detected...)
	evidence = append(evidence, runtimeCreate...)
	confidence := ConfidenceDetected
	if len(evidence) == 0 {
		evidence = suggested
		confidence = ConfidenceSuggested
	}
	if len(evidence) == 0 {
		return nil, nil
	}
	finding := Finding{
		Capability: "object-storage.s3",
		Direction:  DirectionConsume,
		Confidence: confidence,
		Evidence:   uniqueEvidence(evidence),
	}
	if len(runtimeCreate) > 0 {
		finding.Operations = []RuntimeOperation{RuntimeCreate}
	}
	return []Finding{finding}, nil
}

type openMetricsDetector struct{}

func (openMetricsDetector) Name() string { return "metrics" }

func (openMetricsDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	var detected, suggested []Evidence
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		base := strings.ToLower(filepath.Base(path))
		lower := strings.ToLower(string(data))
		if isSourceFile(base) || strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".toml") {
			if strings.Contains(lower, "/metrics") {
				detected = append(detected, Evidence{
					Kind: EvidenceEndpoint, Path: path,
					Detail: "repository exposes or references the conventional /metrics endpoint",
				})
			}
		}
		if isDependencyFile(base) && containsAnyToken(lower, []string{
			"prometheus/client_golang", "prometheus_client", "prom-client", "micrometer-registry-prometheus",
		}) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceDependency, Path: path,
				Detail: "dependency metadata references an OpenMetrics/Prometheus client",
			})
		}
		if isSourceFile(base) && containsAny(lower, []string{
			"promhttp.handler", "prometheus_client", "prom-client", "generate_latest(",
		}) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceImport, Path: path,
				Detail: "source references an OpenMetrics/Prometheus instrumentation library",
			})
		}
	}
	if len(detected) > 0 {
		return []Finding{{
			Capability: "metrics", Direction: DirectionProvide,
			Confidence: ConfidenceDetected, Evidence: uniqueEvidence(append(detected, suggested...)),
		}}, nil
	}
	if len(suggested) > 0 {
		return []Finding{{
			Capability: "metrics", Direction: DirectionProvide,
			Confidence: ConfidenceSuggested, Evidence: uniqueEvidence(suggested),
		}}, nil
	}
	return nil, nil
}

type otlpDetector struct{}

func (otlpDetector) Name() string { return "telemetry.otlp" }

func (otlpDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	var detected, suggested []Evidence
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		base := strings.ToLower(filepath.Base(path))
		lower := strings.ToLower(string(data))
		if isEnvFile(base) {
			for _, name := range readEnvNames(data) {
				if strings.HasPrefix(strings.ToUpper(name), "OTEL_EXPORTER_OTLP_") || strings.EqualFold(name, "OTEL_EXPORTER_OTLP_ENDPOINT") {
					detected = append(detected, Evidence{Kind: EvidenceEnv, Path: path, Detail: "variable " + name})
				}
			}
		}
		if isDependencyFile(base) && containsAnyToken(lower, []string{
			"opentelemetry", "otel-exporter", "otlp",
		}) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceDependency, Path: path,
				Detail: "dependency metadata references OpenTelemetry/OTLP",
			})
		}
		if isSourceFile(base) && containsAny(lower, []string{
			"otlptrace", "otlpmetric", "otlplog", "opentelemetry", "otel.exporter",
		}) {
			suggested = append(suggested, Evidence{
				Kind: EvidenceImport, Path: path,
				Detail: "source references OpenTelemetry/OTLP export",
			})
		}
	}
	if len(detected) > 0 {
		return []Finding{{
			Capability: "telemetry.otlp", Direction: DirectionExport,
			Confidence: ConfidenceDetected, Evidence: uniqueEvidence(append(detected, suggested...)),
		}}, nil
	}
	if len(suggested) > 0 {
		return []Finding{{
			Capability: "telemetry.otlp", Direction: DirectionExport,
			Confidence: ConfidenceSuggested, Evidence: uniqueEvidence(suggested),
		}}, nil
	}
	return nil, nil
}
