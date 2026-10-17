package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// appEditPreviewAuthority projects desired intent through the existing read-only
// footprint authority. It must not persist source, preferences or provider state.
type appEditPreviewAuthority func(context.Context, appEditDesired) (GuidedReadContext, error)

// appEditExecuteAuthority is the existing mutation/lifecycle boundary supplied
// by the caller. Edit never performs provider/runtime mutations itself.
type appEditExecuteAuthority func(context.Context, appEditDesired, application.ProviderImpactPlan, application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error)

type appEditAuthorities struct {
	Read      appMachineRead
	Choices   appMachineChoiceReader
	Plan      appMachineDesiredPlanner
	Preview   appEditPreviewAuthority
	Authorize appMachineAuthorize
	Execute   appEditExecuteAuthority
}

type appEditSession struct {
	Loaded     appEditLoaded
	Controller appWizardController
}

// appEditStart hydrates the exact authored source and initializes the accepted
// controller from current named requirements and Target-local preferences.
// No mutation authority is reachable while this function runs.
func appEditStart(ctx context.Context, target string, source []byte, snapshot application.ResolutionSnapshot, authorities appEditAuthorities) (appEditSession, error) {
	if authorities.Read == nil || authorities.Choices == nil || authorities.Plan == nil || authorities.Preview == nil {
		return appEditSession{}, errors.New("app edit requires read, choice, plan and preview authorities")
	}
	loaded, err := appMachineHydrate(ctx, target, source, appEditLoadReader(authorities.Read))
	if err != nil {
		return appEditSession{}, err
	}
	if snapshot.Target != target {
		return appEditSession{}, fmt.Errorf("edit snapshot Target %q differs from hydrated Target %q", snapshot.Target, target)
	}
	manifest, err := appEditParseSource(loaded)
	if err != nil {
		return appEditSession{}, err
	}
	controller, err := newAppWizardController(ctx, appWizardInput{
		Manifest: manifest,
		Snapshot: snapshot,
		Selection: appCapabilitySelectionInput{
			Current: loaded.Capabilities,
		},
		TTY: true,
	})
	if err != nil {
		return appEditSession{}, err
	}

	// Preserve existing explicit preferences through the same accepted selector
	// used for a new choice. Automatic preferences remain automatic.
	for _, preference := range loaded.Preferences {
		if preference.InstanceID == "" {
			continue
		}
		controller, err = controller.SelectProvider(ctx, preference.Requirement, preference.InstanceID)
		if err != nil {
			return appEditSession{}, fmt.Errorf("restore provider preference for %s/%s: %w", preference.Requirement.Kind, preference.Requirement.Name, err)
		}
	}

	controller.refresh = func(refreshCtx context.Context, decision appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
		fresh, err := authorities.Read(refreshCtx, decision.Target)
		if err != nil {
			return nil, nil, err
		}
		desired, err := appEditDesiredFromDecision(loaded, decision)
		if err != nil {
			return nil, nil, err
		}
		request := appEditMachineRequest(decision, fresh.SnapshotRevision)
		proposal, err := appMachinePropose(refreshCtx, request, authorities.Read, authorities.Choices, authorities.Plan)
		if err != nil {
			return nil, nil, err
		}
		preview, err := authorities.Preview(refreshCtx, desired)
		if err != nil {
			return nil, nil, err
		}
		if preview.Target != decision.Target || preview.Footprint.Target != decision.Target || preview.ApplicationID != loaded.ApplicationID {
			return nil, nil, errors.New("desired preview differs from hydrated application owner")
		}
		return &preview, &proposal.Plan, nil
	}

	return appEditSession{Loaded: loaded, Controller: controller}, nil
}

func appEditMachineRequest(decision appWizardDecision, snapshotRevision string) appMachineDesiredRequest {
	request := appMachineDesiredRequest{
		Target:           decision.Target,
		SnapshotRevision: snapshotRevision,
		Requirements:     append([]capability.Requirement(nil), decision.Requirements...),
	}
	for _, preference := range decision.Preferences {
		request.Choices = append(request.Choices, appMachineDesiredChoice{
			Requirement: preference.Requirement,
			InstanceID:  preference.InstanceID,
		})
	}
	return request
}
