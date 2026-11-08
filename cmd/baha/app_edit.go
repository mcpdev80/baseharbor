package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/deployment"
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
func appEditStart(ctx context.Context, target string, source []byte, snapshot application.ResolutionSnapshot, authorities appEditAuthorities, out io.Writer) (appEditSession, error) {
	if out == nil {
		out = io.Discard
	}
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
	// used for a new choice. Automatic preferences remain automatic. A stored
	// instance that the current projection lost must never block the whole
	// edit: it is re-resolved against the live Target provider inventory and
	// kept; an instance that exists nowhere anymore reverts to automatic with
	// a visible line naming it and the re-resolution step.
	for _, preference := range loaded.Preferences {
		if preference.InstanceID == "" {
			continue
		}
		if controller, err = selectStoredProviderPreference(ctx, controller, preference); err != nil {
			if !errors.Is(err, errAppEditPreferenceUnresolvable) {
				return appEditSession{}, fmt.Errorf("restore provider preference for %s/%s: %w", preference.Requirement.Kind, preference.Requirement.Name, err)
			}
			fmt.Fprintf(out, "provider preference %s/%s instance %s is unavailable on Target %s; the choice reverted to automatic — re-resolve it with 'provider %s/%s <id>' or keep automatic to select from the current Target inventory\n",
				preference.Requirement.Kind, preference.Requirement.Name, preference.InstanceID, target,
				preference.Requirement.Kind, preference.Requirement.Name)
			if controller, err = controller.SelectProvider(ctx, preference.Requirement, ""); err != nil {
				return appEditSession{}, fmt.Errorf("reset unavailable provider preference for %s/%s: %w", preference.Requirement.Kind, preference.Requirement.Name, err)
			}
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

// errAppEditPreferenceUnresolvable marks the single case where neither the
// projection nor the live provider registry can resolve an explicit stored
// preference. It is reported to the operator, never a silent failure.
var errAppEditPreferenceUnresolvable = errors.New("provider preference instance is not resolvable on this Target")

// liveProviderKindsForInstance returns the distinct provider kinds the live
// provider registry registers under one instance identity.
func liveProviderKindsForInstance(instanceID string) ([]capability.ProviderKind, error) {
	dataRoot, err := deployment.DataRoot()
	if err != nil {
		return nil, err
	}
	registry, err := (capability.RegistryStore{Path: filepath.Join(dataRoot, "provider-registry.json")}).Load()
	if err != nil {
		return nil, err
	}
	var kinds []capability.ProviderKind
	for _, instance := range registry.Instances {
		if instance.ID != instanceID {
			continue
		}
		known := false
		for _, kind := range kinds {
			known = known || kind == instance.Provider.Kind
		}
		if !known {
			kinds = append(kinds, instance.Provider.Kind)
		}
	}
	return kinds, nil
}

// selectStoredProviderPreference restores one explicit stored preference via
// the accepted selector. When the current projection lost the instance, the
// live Target registry re-offers it (re-resolution against the running
// provider) and the same choice is retried on the widened snapshot.
func selectStoredProviderPreference(ctx context.Context, controller appWizardController, preference application.ProviderPreference) (appWizardController, error) {
	if preference.Requirement.Kind == "" || preference.Requirement.Name == "" {
		// A stored preference without its requirement identity can never be
		// re-resolved; it stays unselected instead of blocking the session.
		return controller, errAppEditPreferenceUnresolvable
	}
	if controller, err := controller.SelectProvider(ctx, preference.Requirement, preference.InstanceID); err == nil {
		return controller, nil
	}
	// The projection lost this instance. An unapplied record may still store
	// the preference as requirement-only (the provider kind the canonical
	// owner would pin was never persisted); recover it from the live Target
	// inventory so re-resolution sees the same shape the plan authority does.
	probe := preference
	if probe.ProviderKind == "" {
		if kinds, kindErr := liveProviderKindsForInstance(preference.InstanceID); kindErr == nil && len(kinds) == 1 {
			probe.ProviderKind = kinds[0]
		}
	}
	offered, err := liveCanonicalChoiceInstances([]application.ProviderPreference{probe})
	if err != nil {
		return controller, err
	}
	extra := offered[preference.Requirement]
	if len(extra) == 0 {
		return controller, errAppEditPreferenceUnresolvable
	}
	widened := controller.snapshot
	widened.Registry.Instances = append(append([]capability.ProviderInstance(nil), widened.Registry.Instances...), extra...)
	previous := controller.snapshot
	controller.snapshot = widened
	next, selectErr := controller.SelectProvider(ctx, preference.Requirement, preference.InstanceID)
	if selectErr != nil {
		// The widened snapshot must not leak into a failing session.
		controller.snapshot = previous
		return controller, errAppEditPreferenceUnresolvable
	}
	return next, nil
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
// <id>". The line terminator is stripped but spaces are preserved, so a literal
// space toggles while a bare Enter advances. EOF, cancel and repeated No stay
// write-free (core-916); Enter previews a fresh impact and never confirms.
func driveAppEditWizard(ctx context.Context, session appEditSession, reader *bufio.Reader, out io.Writer) (appEditSession, error) {
	for session.Controller.phase != appWizardDecided && session.Controller.phase != appWizardAborted {
		if err := ctx.Err(); err != nil {
			session.Controller, _ = session.Controller.Step(ctx, appWizardCancel)
			return session, err
		}
		fmt.Fprintln(out, session.Controller.View())
		line, readErr := reader.ReadString('\n')
		key := strings.ToLower(strings.TrimRight(line, "\r\n"))
		if readErr != nil && key == "" {
			session.Controller, _ = session.Controller.Step(ctx, appWizardEOF)
			return session, errAppWizardAbort
		}
		switch session.Controller.phase {
		case appWizardNeeds:
			session = driveAppEditNeeds(ctx, session, key, out)
		case appWizardPreview:
			next, err := driveAppEditPreview(ctx, session, key)
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
func driveAppEditNeeds(ctx context.Context, session appEditSession, key string, out io.Writer) appEditSession {
	switch key {
	case "", "next", "enter":
		next, _ := session.Controller.Step(ctx, appWizardNext)
		session.Controller = next
		return session
	case "c", "cancel", "q", "quit":
		session.Controller, _ = session.Controller.Step(ctx, appWizardCancel)
		return session
	case " ", "space":
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
	if strings.HasPrefix(key, "provider ") {
		fields := strings.Fields(strings.TrimPrefix(key, "provider "))
		if len(fields) == 2 {
			if requirement, ok := parseAppEditRequirementRef(fields[0]); ok {
				instanceID := fields[1]
				if instanceID == "automatic" {
					instanceID = ""
				}
				next, err := session.Controller.SelectProvider(ctx, requirement, instanceID)
				if err == nil {
					session.Controller = next
					return session
				}
				fmt.Fprintf(out, "provider selection failed: %v\n", err)
				return session
			}
		}
		fmt.Fprintln(out, "usage: provider <kind/name> <instance-id|automatic>")
		return session
	}
	fmt.Fprintln(out, "use space/up/down to edit needs, 'provider <kind/name> <id|automatic>' to choose a provider, enter to preview, or cancel to quit without changes")
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
	session, err := appEditStart(ctx, resolved.Target.Name, source, snapshot, authorities, out)
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
