package repositoryinspect

import (
	"context"
	"path/filepath"
	"strings"
)

type secretsDetector struct{}

func (secretsDetector) Name() string { return "secrets" }

func (secretsDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	var evidence []Evidence
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isEnvFile(strings.ToLower(filepath.Base(path))) {
			continue
		}
		for _, name := range readEnvNames(data) {
			if likelySecretName(name) {
				evidence = append(evidence, Evidence{Kind: EvidenceEnv, Path: path, Detail: "secret input " + name})
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

type httpExposureDetector struct{}

func (httpExposureDetector) Name() string { return "exposure.http" }

func (httpExposureDetector) Detect(ctx context.Context, snapshot Snapshot) ([]Finding, error) {
	var evidence []Evidence
	for path, data := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isComposeFile(strings.ToLower(filepath.Base(path))) {
			continue
		}
		services, err := detectComposeServices(data)
		if err != nil {
			return nil, err
		}
		for _, service := range services {
			if service.HasPorts && (service.WorkloadProtocol == "http" || service.WorkloadProtocol == "https") {
				evidence = append(evidence, Evidence{
					Kind:   EvidenceEndpoint,
					Path:   path,
					Detail: "compose workload " + service.Name + " declares " + service.WorkloadProtocol + " protocol and published port",
				})
			}
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
