package main

import (
	"context"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/runtimebroker"
)

type runtimeArtifactObservation struct {
	Reference       string `json:"reference,omitempty"`
	ImageID         string `json:"image_id,omitempty"`
	Digest          string `json:"digest,omitempty"`
	ExpectedVersion string `json:"expected_version,omitempty"`
	Detail          string `json:"detail,omitempty"`
}

type applicationStatusResult struct {
	application.StatusResult
	TLS             *applicationTLSObservation `json:"tls,omitempty"`
	RuntimeArtifact *runtimeArtifactObservation `json:"runtime_artifact,omitempty"`
	RuntimeDocsURL  string                      `json:"runtime_docs_url,omitempty"`

	tlsStatus *applicationTLSStatus
	tlsErr    error
}

func collectApplicationStatusResult(ctx context.Context, store application.Store, args []string) (applicationStatusResult, error) {
	result, err := collectApplicationStatus(ctx, store, args)
	if err != nil {
		return applicationStatusResult{}, err
	}
	resolved, err := resolveApplication(store, args, "status")
	if err != nil {
		return applicationStatusResult{}, err
	}
	tlsStatus, tlsObservation, tlsErr := collectApplicationTLSObservation(resolved)
	if tlsErr != nil || (tlsObservation != nil && !tlsObservation.Healthy) {
		result.Ready = false
	}

	var runtimeArtifact *runtimeArtifactObservation
	var runtimeDocsURL string
	if application.RequiresRuntimeBroker(resolved.Manifest) && result.State != "not_applied" {
		if files, filesErr := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest); filesErr == nil {
			if brokerFiles, brokerErr := runtimebroker.Existing(files); brokerErr == nil {
				runtimeDocsURL = strings.TrimSpace(brokerFiles.DocsURL)
				runtimeArtifact = &runtimeArtifactObservation{
					Reference:       strings.TrimSpace(brokerFiles.Image),
					ExpectedVersion: strings.TrimSpace(version),
				}
				if compose, composeErr := bhruntime.DetectCompose(ctx); composeErr == nil {
					identity, identityErr := compose.ProjectServiceImageIdentity(ctx, runtimebroker.ProjectName(resolved.Manifest), runtimebroker.ServiceName)
					if identityErr != nil {
						runtimeArtifact.Detail = identityErr.Error()
					} else {
						runtimeArtifact.Reference = identity.Reference
						runtimeArtifact.ImageID = identity.ImageID
						runtimeArtifact.Digest = identity.Digest
					}
				} else {
					runtimeArtifact.Detail = composeErr.Error()
				}
			}
		}
	}
	return applicationStatusResult{
		StatusResult:    result,
		TLS:             tlsObservation,
		RuntimeArtifact: runtimeArtifact,
		RuntimeDocsURL:  runtimeDocsURL,
		tlsStatus:       tlsStatus,
		tlsErr:          tlsErr,
	}, nil
}
