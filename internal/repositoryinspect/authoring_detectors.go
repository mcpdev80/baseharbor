package repositoryinspect

import (
	"context"
	"path/filepath"
	"strings"
)

type httpExposureDetector struct{}

func (httpExposureDetector) Name() string { return "exposure.http" }

func (httpExposureDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	var evidence []Evidence
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		base := strings.ToLower(filepath.Base(path))
		if !isSourceFile(base) && base != "dockerfile" {
			continue
		}
		lower := strings.ToLower(string(data))
		if containsAny(lower, []string{
			"http.listenandserve",
			"http.newservemux",
			"http.handlefunc",
			"http.server",
			"fastapi(",
			"express(",
			"next/server",
			"quarkus.http",
			"jakarta.ws.rs",
			"@path(",
		}) {
			evidence = append(evidence, Evidence{
				Kind:   EvidenceImport,
				Path:   path,
				Detail: "source exposes an HTTP application endpoint",
			})
		}
		if base == "dockerfile" && strings.Contains(lower, "expose ") {
			evidence = append(evidence, Evidence{
				Kind:   EvidencePort,
				Path:   path,
				Detail: "Dockerfile exposes an application port",
			})
		}
	}
	if len(evidence) == 0 {
		return nil, nil
	}
	return []Finding{{
		Capability: "exposure.http",
		Direction:  DirectionProvide,
		Confidence: ConfidenceDetected,
		Evidence:   uniqueEvidence(evidence),
	}}, nil
}

type secretsDetector struct{}

func (secretsDetector) Name() string { return "secrets" }

func (secretsDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	var evidence []Evidence
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		base := strings.ToLower(filepath.Base(path))
		if isEnvFile(base) {
			for _, name := range readEnvNames(data) {
				if likelySecretName(name) {
					evidence = append(evidence, Evidence{
						Kind:   EvidenceEnv,
						Path:   path,
						Detail: "secret binding " + name,
					})
				}
			}
		}
		if isSourceFile(base) {
			lower := strings.ToLower(string(data))
			if containsAny(lower, []string{
				"os.getenv(",
				"os.environ[",
				"process.env.",
				"system.getenv(",
			}) && containsAny(lower, []string{"secret", "password", "token", "api_key", "private_key"}) {
				evidence = append(evidence, Evidence{
					Kind:   EvidenceImport,
					Path:   path,
					Detail: "source consumes secret-shaped environment bindings",
				})
			}
		}
	}
	if len(evidence) == 0 {
		return nil, nil
	}
	return []Finding{{
		Capability: "secrets",
		Direction:  DirectionConsume,
		Confidence: ConfidenceDetected,
		Evidence:   uniqueEvidence(evidence),
	}}, nil
}
