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

		if isComposeFile(base) {
			for _, service := range detectComposeServices(data) {
				if service.ObjectStorage {
					detected = append(detected, Evidence{
						Kind: EvidenceCompose, Path: path,
						Detail: "compose service " + service.Name + " provides S3-compatible object storage",
					})
				}
			}
		}

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
	var genericDetected, genericSuggested []Evidence
	detectedSignals := map[string][]Evidence{}
	suggestedSignals := map[string][]Evidence{}
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		base := strings.ToLower(filepath.Base(path))
		lower := strings.ToLower(string(data))
		if isEnvFile(base) {
			for _, name := range readEnvNames(data) {
				upper := strings.ToUpper(name)
				evidence := Evidence{Kind: EvidenceEnv, Path: path, Detail: "variable " + name}
				switch upper {
				case "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT":
					detectedSignals["traces"] = append(detectedSignals["traces"], evidence)
				case "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT":
					detectedSignals["metrics"] = append(detectedSignals["metrics"], evidence)
				case "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT":
					detectedSignals["logs"] = append(detectedSignals["logs"], evidence)
				case "OTEL_EXPORTER_OTLP_ENDPOINT":
					genericDetected = append(genericDetected, evidence)
				}
			}
		}
		if isDependencyFile(base) && containsAnyToken(lower, []string{
			"opentelemetry", "otel-exporter", "otlp",
		}) {
			genericSuggested = append(genericSuggested, Evidence{
				Kind: EvidenceDependency, Path: path,
				Detail: "dependency metadata references OpenTelemetry/OTLP",
			})
		}
		if isSourceFile(base) {
			if strings.Contains(lower, "otlptrace") {
				suggestedSignals["traces"] = append(suggestedSignals["traces"], Evidence{
					Kind: EvidenceImport, Path: path, Detail: "source references OTLP trace export",
				})
			}
			if strings.Contains(lower, "otlpmetric") {
				suggestedSignals["metrics"] = append(suggestedSignals["metrics"], Evidence{
					Kind: EvidenceImport, Path: path, Detail: "source references OTLP metric export",
				})
			}
			if strings.Contains(lower, "otlplog") {
				suggestedSignals["logs"] = append(suggestedSignals["logs"], Evidence{
					Kind: EvidenceImport, Path: path, Detail: "source references OTLP log export",
				})
			}
			if containsAny(lower, []string{"opentelemetry", "otel.exporter"}) {
				genericSuggested = append(genericSuggested, Evidence{
					Kind: EvidenceImport, Path: path,
					Detail: "source references OpenTelemetry/OTLP export",
				})
			}
		}
	}

	var findings []Finding
	for _, signal := range []string{"traces", "metrics", "logs"} {
		if evidence := detectedSignals[signal]; len(evidence) > 0 {
			findings = append(findings, Finding{
				Capability: "telemetry.otlp", Name: signal, Direction: DirectionExport,
				Confidence: ConfidenceDetected, Evidence: uniqueEvidence(evidence),
			})
		} else if evidence := suggestedSignals[signal]; len(evidence) > 0 {
			findings = append(findings, Finding{
				Capability: "telemetry.otlp", Name: signal, Direction: DirectionExport,
				Confidence: ConfidenceSuggested, Evidence: uniqueEvidence(evidence),
			})
		}
	}
	if len(genericDetected) > 0 {
		findings = append(findings, Finding{
			Capability: "telemetry.otlp", Direction: DirectionExport,
			Confidence: ConfidenceDetected,
			Evidence:   uniqueEvidence(append(genericDetected, genericSuggested...)),
		})
	} else if len(genericSuggested) > 0 && len(findings) == 0 {
		findings = append(findings, Finding{
			Capability: "telemetry.otlp", Direction: DirectionExport,
			Confidence: ConfidenceSuggested, Evidence: uniqueEvidence(genericSuggested),
		})
	}
	return findings, nil
}

type runtimeAPIDetector struct{}

func (runtimeAPIDetector) Name() string { return "runtime-api" }

func (runtimeAPIDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	var evidence []Evidence
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		base := strings.ToLower(filepath.Base(path))
		lower := strings.ToLower(string(data))
		if isEnvFile(base) {
			for _, name := range readEnvNames(data) {
				if strings.HasPrefix(strings.ToUpper(name), "BASEHARBOR_RUNTIME_") {
					evidence = append(evidence, Evidence{
						Kind: EvidenceEnv, Path: path, Detail: "variable " + name,
					})
				}
			}
		}
		if isSourceFile(base) && (strings.Contains(lower, "/runtime/v1/") || strings.Contains(lower, "baseharbor_runtime_")) {
			evidence = append(evidence, Evidence{
				Kind: EvidenceCall, Path: path,
				Detail: "source references the BaseHarbor Runtime API",
			})
		}
	}
	if len(evidence) == 0 {
		return nil, nil
	}
	return []Finding{{
		Capability: "runtime-api",
		Direction:  DirectionConsume,
		Confidence: ConfidenceDetected,
		Evidence:   uniqueEvidence(evidence),
	}}, nil
}
