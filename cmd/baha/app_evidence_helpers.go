package main

import (
	"context"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationbackup"
	"github.com/mcpdev80/baseharbor/internal/evidence"
)

func recoveryContributorMetadata(manifest applicationbackup.RecoveryManifest, verifiedSelected bool) []application.RecoveryContributorMetadata {
	result := make([]application.RecoveryContributorMetadata, 0, len(manifest.Contributors))
	for _, contributor := range manifest.Contributors {
		result = append(result, application.RecoveryContributorMetadata{
			StateClass:         string(contributor.StateClass),
			LogicalResource:    contributor.LogicalResource,
			Ownership:          contributor.Ownership,
			Support:            string(contributor.Support),
			Selected:           contributor.Selected,
			Durable:            contributor.Durable,
			ExplicitlyExcluded: contributor.ExplicitlyExcluded,
			Verified:           verifiedSelected && contributor.Selected && contributor.Support == applicationbackup.RecoverySupported,
			Reason:             contributor.Reason,
		})
	}
	return result
}

func recordApplicationAudit(ctx context.Context, resolved resolvedApplication, operation, outcome, verification, detail string) error {
	event := evidence.NewAuditEvent(
		ctx,
		resolved.Target.Name,
		resolved.Manifest.Name,
		resolved.Manifest.Environment,
		operation,
		outcome,
	)
	event.LifecycleResult = outcome
	event.VerificationResult = verification
	event.Ownership = "baseharbor"
	event.Detail = detail
	if err := evidence.Append(resolved.TargetStateRoot, event); err != nil {
		return fmt.Errorf("record application audit evidence: %w", err)
	}
	return nil
}

func selectedRecoveryResources(manifest applicationbackup.RecoveryManifest, class applicationbackup.RecoveryStateClass) []string {
	var result []string
	for _, contributor := range manifest.Contributors {
		if contributor.StateClass == class && contributor.Selected && contributor.Support == applicationbackup.RecoverySupported {
			result = append(result, contributor.LogicalResource)
		}
	}
	return result
}
