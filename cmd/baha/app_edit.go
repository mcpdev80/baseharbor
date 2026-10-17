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

// driveAppEditWizard runs the accepted controller over line-oriented input,
// editing needs with the selection keys and providers with "provider <kind/name>
// <id>". EOF, cancel and repeated No stay write-free (core-916); Enter previews a
// fresh impact and never confirms execution.
func driveAppEditWizard(ctx context.Context, session appEditSession, reader *bufio.Reader, out io.Writer) (appEditSession, error) {
	for session.Controller.phase != appWizardDecided && session.Controller.phase != appWizardAborted {
		if err := ctx.Err(); err != nil {
			session.Controller, _ = session.Controller.Step(ctx, appWizardCancel)
			return session, err
		}
		fmt.Fprintln(out, session.Controller.View())
		line, readErr := reader.ReadString('\n')
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if readErr != nil && trimmed == "" {
			session.Controller, _ = session.Controller.Step(ctx, appWizardEOF)
			return session, errAppWizardAbort
		}
		switch session.Controller.phase {
		case appWizardNeeds:
			session = driveAppEditNeeds(ctx, session, trimmed, out)
		case appWizardPreview:
			next, err := driveAppEditPreview(ctx, session, trimmed)
			session.Controller = next
			if err != nil {
				return session, err
			}
		}
	}
	return session, nil
}

// driveAppEditNeeds applies a selection, provider-choice or navigation command on
// the needs screen. It never confirms a decision or reaches execution.
func driveAppEditNeeds(ctx context.Context, session appEditSession, trimmed string, out io.Writer) appEditSession {
	switch trimmed {
	case "", "next", "enter":
		next, _ := session.Controller.Step(ctx, appWizardNext)
		session.Controller = next
		return session
	case "c", "cancel", "q", "quit":
		session.Controller, _ = session.Controller.Step(ctx, appWizardCancel)
		return session
	case "space", " ":
		next, _ := session.Controller.SelectNeed(ctx, "space")
		session.Controller = next
		return session
	case "up", "k":
		next, _ := session.Controller.SelectNeed(ctx, "up")
		session.Controller = next
		return session
	case "down", "j":
		next, _ := session.Controller.SelectNeed(ctx, "down")
		session.Controller = next
		return session
	case "n", "no", "back", "b":
		fmt.Fprintln(out, "already on the needs screen; adjust the checklist or press enter to preview")
		return session
	}
	if strings.HasPrefix(trimmed, "provider ") {
		fields := strings.Fields(strings.TrimPrefix(trimmed, "provider "))
		if len(fields) == 2 {
			if requirement, ok := parseAppEditRequirementRef(fields[0]); ok {
				next, err := session.Controller.SelectProvider(ctx, requirement, fields[1])
				if err == nil {
					session.Controller = next
					return session
				}
				fmt.Fprintf(out, "provider selection failed: %v\n", err)
				return session
			}
		}
		fmt.Fprintln(out, "usage: provider <kind/name> <instance-id>")
		return session
	}
	fmt.Fprintln(out, "use space/up/down to edit needs, 'provider <kind/name> <id>' to choose a provider, enter to preview, or cancel to quit without changes")
	return session
}

// driveAppEditPreview confirms or returns from the read-only preview. Enter and
// no/back return to the needs screen with a fresh preview on the next advance.
func driveAppEditPreview(ctx context.Context, session appEditSession, trimmed string) (appWizardController, error) {
	switch trimmed {
	case "y", "yes":
		return session.Controller.Step(ctx, appWizardYes)
	case "", "n", "no", "b", "back":
		return session.Controller.Step(ctx, appWizardBack)
	case "c", "cancel", "q", "quit":
		next, _ := session.Controller.Step(ctx, appWizardCancel)
		return next, errAppWizardAbort
	default:
		return session.Controller, nil
	}
}

// parseAppEditRequirementRef parses a "kind/name" requirement reference.
func parseAppEditRequirementRef(ref string) (capability.Requirement, bool) {
	kind, name, found := strings.Cut(ref, "/")
	if !found || kind == "" || name == "" {
		return capability.Requirement{}, false
	}
	return capability.Requirement{Kind: capability.Kind(kind), Name: name}, true
}

// runAppEdit is the callable end-to-end Edit handler. It assembles the
// production lifecycle and source connection, hydrates the current selection,
// drives input to a confirmed or aborted decision and applies only through the
// confirmed consequence path. Central command/MCP registration stays with core-921.
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
	// One buffered reader carries navigation and consequence consent so already
	// buffered consent bytes are not lost between phases.
	reader := bufio.NewReader(in)
	session, driveErr := driveAppEditWizard(ctx, session, reader, out)
	if driveErr != nil {
		if _, confirmed := session.Controller.Decision(); !confirmed {
			return appEditApplyResult{Source: appEditLoadSource(session.Loaded), Presentation: "Cancelled; no application changes.\n"}, driveErr
		}
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, driveErr
	}
	return runAppEditCommand(ctx, session, reader, out, authorities)
}
