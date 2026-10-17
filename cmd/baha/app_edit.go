package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

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

// driveAppEditWizard runs the accepted controller over line-oriented input.
// EOF and cancel stay write-free (core-916 contract); the preview phase needs
// an explicit yes before it can confirm a decision.
func driveAppEditWizard(ctx context.Context, session appEditSession, reader *bufio.Reader, out io.Writer) (appEditSession, error) {
	for session.Controller.phase != appWizardDecided && session.Controller.phase != appWizardAborted {
		if err := ctx.Err(); err != nil {
			session.Controller, _ = session.Controller.Step(ctx, appWizardCancel)
			return session, err
		}
		fmt.Fprintln(out, session.Controller.View())
		line, readErr := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "next":
			if readErr != nil && line == "" {
				session.Controller, _ = session.Controller.Step(ctx, appWizardEOF)
				return session, errAppWizardAbort
			}
			event := appWizardNext
			if session.Controller.phase == appWizardPreview {
				event = appWizardBack // Enter never confirms a preview; return to needs
			}
			next, err := session.Controller.Step(ctx, event)
			session.Controller = next
			if err != nil {
				return session, err
			}
		case "y", "yes":
			next, err := session.Controller.Step(ctx, appWizardYes)
			session.Controller = next
			if err != nil {
				return session, err
			}
		case "n", "no", "b", "back":
			next, err := session.Controller.Step(ctx, appWizardNo)
			session.Controller = next
			if err != nil {
				return session, err
			}
		case "c", "cancel", "q", "quit":
			session.Controller, _ = session.Controller.Step(ctx, appWizardCancel)
			return session, errAppWizardAbort
		default:
			if readErr != nil {
				session.Controller, _ = session.Controller.Step(ctx, appWizardEOF)
				return session, errAppWizardAbort
			}
			fmt.Fprintln(out, "unrecognized edit navigation; use enter to continue, y to confirm, n to go back, or cancel to quit without changes")
		}
	}
	return session, nil
}

// runAppEdit is the callable, end-to-end Edit command handler. It assembles the
// production lifecycle and source connection itself (default authorities, the
// real Apply lifecycle and the exact authoritative source bytes), hydrates the
// current selection, drives interactive input to a confirmed or aborted
// decision and applies only through the confirmed consequence path below.
// Central command/MCP registration stays with core-921; that registration may
// bind directly to this function.
func runAppEdit(ctx context.Context, store application.Store, args []string, in io.Reader, out, errOut io.Writer) (appEditApplyResult, error) {
	resolved, err := resolveApplication(ctx, store, args, "edit")
	if err != nil {
		return appEditApplyResult{}, err
	}
	source, err := appEditSourceBytes(resolved)
	if err != nil {
		return appEditApplyResult{}, err
	}
	authorities := defaultAppEditAuthorities(store, args)
	authorities.Execute = appEditRuntimeExecuteAuthority(store, args, resolved, source, authorities, out, errOut, nil, nil)
	snapshot, _, err := applicationLifecycleSnapshot(resolved, observeApplicationProviderFacts(ctx, resolved))
	if err != nil {
		return appEditApplyResult{}, err
	}
	session, err := appEditStart(ctx, resolved.Target.Name, source, snapshot, authorities)
	if err != nil {
		return appEditApplyResult{}, err
	}
	if in == nil {
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, errAppWizardAbort
	}
	session, driveErr := driveAppEditWizard(ctx, session, bufio.NewReader(in), out)
	if driveErr != nil {
		if _, confirmed := session.Controller.Decision(); !confirmed {
			return appEditApplyResult{Source: appEditLoadSource(session.Loaded), Presentation: "Cancelled; no application changes.\n"}, driveErr
		}
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, driveErr
	}
	return runAppEditCommand(ctx, session, in, out, authorities)
}
