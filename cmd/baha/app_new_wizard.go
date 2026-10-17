package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
)

type greenfieldCapabilityChoice struct {
	kind  capability.Kind
	label string
}

type guidedStackSelection struct {
	RawProfile *development.StackProfile
	Effective  development.StackProfile
	SaveScope  development.ProfileScope
	Created    bool
}

var appNewLifecycleExecutor = executeApplicationApplyLifecycle

func runAppNewWizard(ctx context.Context, out, errOut io.Writer) error {
	reader := bufio.NewReader(appNewInput)
	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		return err
	}
	catalog, err := effectiveDevelopmentProfileCatalog(".", registry)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Create a new application")
	fmt.Fprintln(out)

	name, err := promptLine(reader, out, "Application name", "")
	if err != nil {
		return appNewAbortOnEOF(ctx, out, appWizardController{}, err)
	}
	name = slugifyAppName(name)
	if name == "" {
		return usageError("application name is required", "Enter a lowercase application name such as catalog-api.")
	}

	root, err := guidedNewApplicationRoot(reader, out, name)
	if err != nil {
		return appNewAbortOnEOF(ctx, out, appWizardController{}, err)
	}
	stack, err := guidedStackProfile(reader, out, catalog, registry)
	if err != nil {
		return appNewAbortOnEOF(ctx, out, appWizardController{}, err)
	}
	kinds, secrets, profile, err := guidedGreenfieldCapabilities(reader, out, stack.Effective, registry)
	if err != nil {
		return appNewAbortOnEOF(ctx, out, appWizardController{}, err)
	}
	if stack.Created && stack.RawProfile != nil {
		raw := *stack.RawProfile
		for _, preference := range profile.Capabilities {
			if !profileContainsPreference(stack.Effective, preference) && !profileContainsPreference(raw, preference) {
				raw.Capabilities = append(raw.Capabilities, preference)
			}
		}
		stack.RawProfile = &raw
	}

	request := development.NewApplicationRequest{
		Name: name, Environment: "dev", Profile: &profile, Capabilities: kinds, Secrets: secrets,
	}
	if stack.Created && stack.RawProfile != nil && stack.SaveScope == development.ProfileScopeRepository {
		request.RepositoryStackProfile = stack.RawProfile
	}
	bootstrap, err := development.BootstrapApplication(request, registry)
	if err != nil {
		return err
	}
	request.ApplicationID = bootstrap.Manifest.ApplicationID

	resolved, snapshot, err := appNewWizardAuthorityContext(ctx, root, bootstrap.Manifest)
	if err != nil {
		return err
	}
	requirements, err := appNewManifestRequirements(bootstrap.Manifest)
	if err != nil {
		return err
	}
	controller, err := newAppWizardController(ctx, appWizardInput{
		Manifest: bootstrap.Manifest, Snapshot: snapshot,
		Selection: appCapabilitySelectionInput{Current: requirements},
		TTY:       true,
		Refresh: func(refreshCtx context.Context, decision appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
			var guided GuidedReadContext
			var impact *application.ProviderImpactPlan
			err := withAppNewTargetPreferences(snapshot, decision.Preferences, func() error {
				var refreshErr error
				guided, refreshErr = readGuidedApplicationContext(refreshCtx, resolved)
				if refreshErr != nil {
					return refreshErr
				}
				plan, refreshErr := buildResolvedApplicationPlan(refreshCtx, resolved)
				if refreshErr != nil {
					return refreshErr
				}
				if plan.ProviderPlan.Impact != nil {
					impact = plan.ProviderPlan.Impact
				} else {
					// The accepted plan reports no provider change; retain a
					// read-only empty impact, not an invented mutation.
					impact = &application.ProviderImpactPlan{Target: decision.Target}
				}
				return nil
			})
			if err != nil {
				return nil, nil, err
			}
			return &guided, impact, nil
		},
	})
	if err != nil {
		return err
	}
	controller, err = appNewCollectProviderPreferences(ctx, reader, out, controller)
	if err != nil {
		if errors.Is(err, errAppWizardAbort) || errors.Is(err, io.EOF) {
			fmt.Fprintln(out, "No changes were made.")
			return nil
		}
		return err
	}
	controller, err = controller.Step(ctx, appWizardNext)
	if err != nil {
		return err
	}
	mode, err := appNewExecutionMode(reader, out)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, errAppWizardAbort) {
			_, _ = controller.Step(ctx, appWizardEOF)
			fmt.Fprintln(out, "No changes were made.")
			return nil
		}
		return err
	}

	for {
		fmt.Fprintln(out)
		fmt.Fprintln(out, controller.View())
		question := "Create project files only (does not start containers or deploy providers)?"
		if mode == "run" {
			question = "Create project files and continue into the separately authorized lifecycle?"
		}
		confirm, promptErr := promptYesNo(reader, out, question, true)
		if promptErr != nil {
			if errors.Is(promptErr, io.EOF) {
				_, _ = controller.Step(ctx, appWizardEOF)
				fmt.Fprintln(out, "No changes were made.")
				return nil
			}
			return promptErr
		}
		if !confirm {
			_, _ = controller.Step(ctx, appWizardNo)
			fmt.Fprintln(out, "No changes were made.")
			return nil
		}
		decided, decideErr := controller.Step(ctx, appWizardYes)
		if decideErr == nil {
			controller = decided
			break
		}
		if !appNewStalePreview(decideErr) {
			return decideErr
		}
		controller, _ = controller.Step(ctx, appWizardNo)
		controller, err = controller.Step(ctx, appWizardNext)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "The Target plan changed. Review the refreshed preview and confirm again.")
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	var savedUserProfile string
	if stack.Created && stack.RawProfile != nil && stack.SaveScope == development.ProfileScopeUser {
		savedUserProfile, err = development.SaveProfile(*stack.RawProfile, development.ProfileScopeUser, root)
		if err != nil {
			return err
		}
	}
	result, err := appNewCreateFromWizardDecision(ctx, controller, root, request, registry)
	if err != nil {
		if savedUserProfile != "" {
			_ = os.Remove(savedUserProfile)
		}
		return err
	}
	fmt.Fprintf(out, "Created %s\n", result.Manifest.Name)
	fmt.Fprintf(out, "Location: %s\n", displayUserPath(root))
	fmt.Fprintf(out, "Stack: %s\n", profile.Metadata.Name)
	fmt.Fprintln(out, "Validation: SATISFIED")

	if mode == "create-only" {
		fmt.Fprintln(out, "Next: cd "+displayUserPath(root)+" && baha up")
		return nil
	}
	decision, ok := controller.Decision()
	if !ok {
		return errors.New("confirmed New decision was lost before lifecycle execution")
	}
	err = withAppNewTargetPreferences(snapshot, decision.Preferences, func() error {
		previous, chdirErr := os.Getwd()
		if chdirErr != nil {
			return chdirErr
		}
		if chdirErr = os.Chdir(root); chdirErr != nil {
			return chdirErr
		}
		defer func() { _ = os.Chdir(previous) }()
		return appNewLifecycleExecutor(ctx, application.DefaultStore(), nil, out, errOut)
	})
	if err != nil {
		fmt.Fprintln(errOut, "Project files were created; the authorized lifecycle did not complete.")
		return fmt.Errorf("New lifecycle incomplete after project creation: %w", err)
	}
	fmt.Fprintln(out, "Lifecycle: VERIFIED")
	return nil
}

func appNewWizardAuthorityContext(ctx context.Context, root string, manifest application.Manifest) (resolvedApplication, application.ResolutionSnapshot, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return resolvedApplication{}, application.ResolutionSnapshot{}, err
	}
	stateRoot, err := targetRuntimeStateRoot(target)
	if err != nil {
		return resolvedApplication{}, application.ResolutionSnapshot{}, err
	}
	resolved := resolvedApplication{Target: target, TargetStateRoot: stateRoot, Manifest: manifest, RepositoryRoot: root, FromRepository: true, SourceAvailable: false}
	snapshot, _, err := applicationLifecycleSnapshot(resolved, observeApplicationProviderFacts(ctx, resolved))
	if err != nil {
		return resolvedApplication{}, application.ResolutionSnapshot{}, err
	}
	return resolved, snapshot, nil
}

func appNewManifestRequirements(manifest application.Manifest) ([]capability.Requirement, error) {
	contract, err := application.PortableContractFromManifest(manifest)
	if err != nil {
		return nil, err
	}
	requirements := append([]capability.Requirement(nil), contract.Capabilities...)
	if contract.Secrets.Managed {
		requirements = append(requirements, capability.Requirement{Kind: capability.Secrets, Name: "default"})
	}
	return requirements, nil
}

func appNewCollectProviderPreferences(ctx context.Context, reader *bufio.Reader, out io.Writer, controller appWizardController) (appWizardController, error) {
	choices, err := controller.choicesForSelection()
	if err != nil {
		return controller, err
	}
	if len(choices) == 0 {
		return controller, nil
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Target provider selection")
	for _, choice := range choices {
		fmt.Fprintln(out, choice.View())
		if len(choice.Candidates) == 0 {
			continue
		}
		value, promptErr := promptLine(reader, out, fmt.Sprintf("Provider for %s/%s (automatic or instance id)", choice.Requirement.Kind, choice.Requirement.Name), "automatic")
		if promptErr != nil {
			if errors.Is(promptErr, io.EOF) {
				aborted, _ := controller.Step(ctx, appWizardEOF)
				return aborted, errAppWizardAbort
			}
			return controller, promptErr
		}
		value = strings.TrimSpace(value)
		if strings.EqualFold(value, "cancel") {
			aborted, _ := controller.Step(ctx, appWizardCancel)
			return aborted, errAppWizardAbort
		}
		if value == "" || strings.EqualFold(value, "automatic") {
			continue
		}
		controller, err = controller.SelectProvider(ctx, choice.Requirement, value)
		if err != nil {
			return controller, err
		}
	}
	return controller, nil
}

func appNewExecutionMode(reader *bufio.Reader, out io.Writer) (string, error) {
	value, err := promptLine(reader, out, "After creation (create-only or run)", "create-only")
	if err != nil {
		return "", err
	}
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "create-only", "create":
		return "create-only", nil
	case "run":
		return "run", nil
	case "cancel":
		return "", errAppWizardAbort
	default:
		return "", fmt.Errorf("invalid New execution mode %q", value)
	}
}

func appNewAbortOnEOF(ctx context.Context, out io.Writer, controller appWizardController, err error) error {
	if !errors.Is(err, io.EOF) {
		return err
	}
	if controller.snapshot.Target != "" {
		_, _ = controller.Step(ctx, appWizardEOF)
	}
	fmt.Fprintln(out, "No changes were made.")
	return nil
}

func appNewStalePreview(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "stale preview") || strings.Contains(message, "stale provider choices")
}

type appNewEnvRestore struct {
	name    string
	value   string
	present bool
}

func withAppNewTargetPreferences(snapshot application.ResolutionSnapshot, preferences []appProviderPreference, fn func() error) error {
	if fn == nil {
		return errors.New("New preference execution callback is required")
	}
	instances := map[string]capability.ProviderInstance{}
	for _, instance := range snapshot.Registry.Instances {
		instances[instance.ID] = instance
	}
	placements := map[capability.ProviderKind]capability.ProviderPlacement{}
	for _, preference := range preferences {
		if preference.Target != snapshot.Target {
			return fmt.Errorf("provider preference Target %q differs from preview Target %q", preference.Target, snapshot.Target)
		}
		if preference.InstanceID == "" {
			continue
		}
		instance, ok := instances[preference.InstanceID]
		if !ok {
			return fmt.Errorf("selected provider instance %q is no longer part of the preview inventory", preference.InstanceID)
		}
		placement := capability.ProviderPlacement{Scope: instance.Scope, SharingBoundary: instance.SharingBoundary, Ownership: instance.Ownership, ExternalReference: instance.Reference}
		if previous, exists := placements[instance.Provider.Kind]; exists && !reflect.DeepEqual(previous, placement) {
			return fmt.Errorf("selected requirements require conflicting placements for provider %s", instance.Provider.Kind)
		}
		placements[instance.Provider.Kind] = placement
	}
	var restore []appNewEnvRestore
	set := func(name, value string) error {
		old, present := os.LookupEnv(name)
		restore = append(restore, appNewEnvRestore{name: name, value: old, present: present})
		if value == "" {
			return os.Unsetenv(name)
		}
		return os.Setenv(name, value)
	}
	defer func() {
		for i := len(restore) - 1; i >= 0; i-- {
			item := restore[i]
			if item.present {
				_ = os.Setenv(item.name, item.value)
			} else {
				_ = os.Unsetenv(item.name)
			}
		}
	}()
	for provider, placement := range placements {
		if err := set(application.ProviderScopeEnv(provider), string(placement.Scope)); err != nil {
			return err
		}
		if err := set(application.ProviderSharingBoundaryEnv(provider), placement.SharingBoundary); err != nil {
			return err
		}
		if err := set(application.ProviderExternalReferenceEnv(provider), placement.ExternalReference); err != nil {
			return err
		}
	}
	return fn()
}

// appNewCreateFromWizardDecision bridges the accepted controller decision into
// the existing project authoring authority. Target provider preferences remain
// Target policy and are deliberately not serialized into the portable source.
func appNewCreateFromWizardDecision(ctx context.Context, controller appWizardController, root string, request development.NewApplicationRequest, registry development.Registry) (development.CreationResult, error) {
	if err := ctx.Err(); err != nil {
		return development.CreationResult{}, err
	}
	decision, approved := controller.Decision()
	if !approved {
		return development.CreationResult{}, errors.New("application creation requires a confirmed current wizard decision")
	}
	if decision.Target == "" {
		return development.CreationResult{}, errors.New("Target is required for New preview")
	}
	if request.Name != controller.manifest.Name || request.Environment != controller.manifest.Environment {
		return development.CreationResult{}, errors.New("New request identity or environment differs from confirmed wizard intent")
	}
	if controller.manifest.ApplicationID == "" {
		return development.CreationResult{}, errors.New("confirmed wizard application identity is missing")
	}
	if request.ApplicationID != "" && request.ApplicationID != controller.manifest.ApplicationID {
		return development.CreationResult{}, errors.New("New request application ID differs from confirmed wizard identity")
	}
	request.ApplicationID = controller.manifest.ApplicationID
	confirmedRequirements, err := appNewManifestRequirements(controller.manifest)
	if err != nil {
		return development.CreationResult{}, err
	}
	if !reflect.DeepEqual(confirmedRequirements, decision.Requirements) {
		return development.CreationResult{}, errors.New("confirmed New requirements changed before project creation")
	}
	bootstrap, err := development.BootstrapApplication(request, registry)
	if err != nil {
		return development.CreationResult{}, err
	}
	if bootstrap.Manifest.ApplicationID != controller.manifest.ApplicationID {
		return development.CreationResult{}, errors.New("New bootstrap changed the confirmed application identity")
	}
	var extras []development.GeneratedFile
	for _, file := range bootstrap.Files {
		if file.Path == "catalog-info.yaml" || strings.HasPrefix(file.Path, ".baseharbor/stacks/") {
			extras = append(extras, file)
		}
	}
	project, err := development.BootstrapProject(root, controller.manifest, bootstrap.Profile, registry, extras...)
	if err != nil {
		return development.CreationResult{}, err
	}
	contract, err := application.PortableContractFromManifest(controller.manifest)
	if err != nil {
		return development.CreationResult{}, err
	}
	bootstrap.Manifest = controller.manifest
	bootstrap.Contract = contract
	bootstrap.Plan = project.Plan
	bootstrap.FilePaths = append([]string(nil), project.Files...)
	validation := development.Validation{Satisfied: project.Satisfied, Capabilities: map[capability.Kind]bool{}}
	for _, componentValidation := range project.Validations {
		for kind, satisfied := range componentValidation.Capabilities {
			validation.Capabilities[kind] = validation.Capabilities[kind] || satisfied
		}
		validation.Diagnostics = append(validation.Diagnostics, componentValidation.Diagnostics...)
	}
	return development.CreationResult{BootstrapResult: bootstrap, Validation: validation}, nil
}

func guidedNewApplicationRoot(reader *bufio.Reader, out io.Writer, name string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if empty, err := directoryIsEmpty(cwd); err == nil && empty && filepath.Base(filepath.Clean(cwd)) == name {
		useCurrent, err := promptYesNo(reader, out, "Create in current directory "+displayUserPath(cwd)+"?", true)
		if err != nil {
			return "", err
		}
		if useCurrent {
			return cwd, nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	for {
		parentInput, err := promptDirectoryPathFrom(reader, out, "Project location", home, appNewInput)
		if err != nil {
			return "", err
		}
		parent := strings.TrimSpace(parentInput)
		if parent == "" {
			parent = home
		} else {
			parent, err = expandUserPath(parent)
			if err != nil {
				return "", err
			}
			if !filepath.IsAbs(parent) {
				parent = filepath.Join(home, parent)
			}
		}
		parent, err = filepath.Abs(parent)
		if err != nil {
			return "", err
		}
		root := filepath.Join(parent, name)
		info, statErr := os.Stat(root)
		switch {
		case os.IsNotExist(statErr):
			fmt.Fprintf(out, "Project root: %s\n", displayUserPath(root))
			return root, nil
		case statErr != nil:
			return "", statErr
		case !info.IsDir():
			fmt.Fprintf(out, "%s already exists and is not a directory. Choose another location.\n", displayUserPath(root))
			continue
		}
		empty, err := directoryIsEmpty(root)
		if err != nil {
			return "", err
		}
		if !empty {
			fmt.Fprintf(out, "%s already exists and is not empty. Choose another location.\n", displayUserPath(root))
			continue
		}
		useExisting, err := promptYesNo(reader, out, "Use existing empty directory "+displayUserPath(root)+"?", false)
		if err != nil {
			return "", err
		}
		if useExisting {
			return root, nil
		}
	}
}

func directoryIsEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

func guidedStackProfile(reader *bufio.Reader, out io.Writer, catalog development.ProfileCatalogEntries, registry development.Registry) (guidedStackSelection, error) {
	if name, err := organizationDefaultStack("dev"); err != nil {
		return guidedStackSelection{}, err
	} else if name != "" {
		if _, exists := catalog[name]; !exists {
			return guidedStackSelection{}, fmt.Errorf("organization default stack %q is not available in the effective stack catalog", name)
		}
		resolved, err := development.ResolveStackProfile(name, development.ProfileMap(catalog))
		if err != nil {
			return guidedStackSelection{}, err
		}
		fmt.Fprintf(out, "\nStack: %s (organization default)\n", name)
		return guidedStackSelection{Effective: resolved.Profile}, nil
	}
	names := sortedProfileNames(catalog)
	fmt.Fprintln(out, "\nChoose a stack")
	for i, name := range names {
		entry := catalog[name]
		fmt.Fprintf(out, "  %d. %s (%s)\n", i+1, name, entry.Scope)
	}
	fmt.Fprintf(out, "  %d. Create a new stack...\n", len(names)+1)
	choice, err := promptNumber(reader, out, "Selection", 1, len(names)+1, 1)
	if err != nil {
		return guidedStackSelection{}, err
	}
	if choice <= len(names) {
		name := names[choice-1]
		resolved, err := development.ResolveStackProfile(name, development.ProfileMap(catalog))
		if err != nil {
			return guidedStackSelection{}, err
		}
		return guidedStackSelection{Effective: resolved.Profile}, nil
	}
	return guidedCreateStackProfile(reader, out, catalog, registry)
}

func guidedCreateStackProfile(reader *bufio.Reader, out io.Writer, catalog development.ProfileCatalogEntries, registry development.Registry) (guidedStackSelection, error) {
	fmt.Fprintln(out, "\nCreate stack")
	name, err := promptLine(reader, out, "Stack name", "")
	if err != nil {
		return guidedStackSelection{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return guidedStackSelection{}, fmt.Errorf("stack name is required")
	}
	if _, exists := catalog[name]; exists {
		return guidedStackSelection{}, fmt.Errorf("stack profile %q already exists; app new never mutates reusable profiles", name)
	}

	baseNames := sortedProfileNames(catalog)
	fmt.Fprintln(out, "\nCreate stack from")
	fmt.Fprintln(out, "  1. Empty stack")
	for i, base := range baseNames {
		fmt.Fprintf(out, "  %d. %s\n", i+2, base)
	}
	baseChoice, err := promptNumber(reader, out, "Selection", 1, len(baseNames)+1, 1)
	if err != nil {
		return guidedStackSelection{}, err
	}

	raw := development.StackProfile{
		APIVersion: development.StackProfileAPIVersion,
		Kind:       development.StackProfileKind,
		Metadata:   development.ProfileMetadata{Name: name},
	}
	if baseChoice > 1 {
		raw.Extends = []string{baseNames[baseChoice-2]}
	}

	ids := registry.IDs()
	short := make([]string, len(ids))
	for i, id := range ids {
		short[i] = strings.TrimPrefix(id, "development/")
	}
	defaultComponents := "go"
	if len(raw.Extends) > 0 {
		defaultComponents = ""
	}
	value, err := promptLine(reader, out, "Additional components (comma-separated: "+strings.Join(short, ", ")+")", defaultComponents)
	if err != nil {
		return guidedStackSelection{}, err
	}
	var counts = map[string]int{}
	for _, token := range splitWizardItems(value) {
		adapterID, err := developmentAdapterID(token)
		if err != nil {
			return guidedStackSelection{}, err
		}
		counts[adapterID]++
		suffix := ""
		if counts[adapterID] > 1 {
			suffix = strconv.Itoa(counts[adapterID])
		}
		defaultID := strings.TrimPrefix(adapterID, "development/") + suffix
		if len(raw.Extends) == 0 && len(splitWizardItems(value)) == 1 {
			defaultID = "app"
		}
		componentID, err := promptLine(reader, out, "Component name", defaultID)
		if err != nil {
			return guidedStackSelection{}, err
		}
		componentID = slugifyAppName(componentID)
		if componentID == "" {
			return guidedStackSelection{}, fmt.Errorf("component name is required")
		}
		role, err := promptLine(reader, out, "Role for "+componentID, "application")
		if err != nil {
			return guidedStackSelection{}, err
		}
		raw.Components = append(raw.Components, development.Component{ID: componentID, Role: strings.TrimSpace(role), Adapter: adapterID})
	}
	if len(raw.Components) == 0 && len(raw.Extends) == 0 {
		return guidedStackSelection{}, fmt.Errorf("a stack requires at least one component")
	}

	temp := development.ProfileCatalog{}
	for n, entry := range catalog {
		temp[n] = entry.Profile
	}
	temp[name] = raw
	resolved, err := development.ResolveStackProfile(name, temp)
	if err != nil {
		return guidedStackSelection{}, err
	}

	fmt.Fprintln(out, "\nSave stack for")
	fmt.Fprintln(out, "  1. My user account")
	fmt.Fprintln(out, "  2. This repository/team")
	scopeChoice, err := promptNumber(reader, out, "Selection", 1, 2, 1)
	if err != nil {
		return guidedStackSelection{}, err
	}
	scope := development.ProfileScopeUser
	if scopeChoice == 2 {
		scope = development.ProfileScopeRepository
	}
	return guidedStackSelection{RawProfile: &raw, Effective: resolved.Profile, SaveScope: scope, Created: true}, nil
}

func guidedGreenfieldCapabilities(reader *bufio.Reader, out io.Writer, profile development.StackProfile, registry development.Registry) ([]capability.Kind, []string, development.StackProfile, error) {
	candidates := []greenfieldCapabilityChoice{
		{capability.ExposureHTTP, "HTTP"},
		{capability.SQL, "SQL"},
		{capability.KeyValue, "Cache"},
		{capability.DurableKeyValue, "Durable Key-Value"},
		{capability.DocumentDatabase, "Document Database"},
		{capability.MessagingQueue, "Messaging Queue"},
		{capability.MessagingPubSub, "Messaging Pub/Sub"},
		{capability.MessagingStream, "Messaging Stream"},
		{capability.ObjectStorageS3, "Object Storage"},
		{capability.Secrets, "Managed Secrets"},
		{capability.TelemetryOTLP, "Traces / OTLP"},
	}
	var available []greenfieldCapabilityChoice
	for _, candidate := range candidates {
		if profileSupportsKind(profile, registry, candidate.kind) {
			available = append(available, candidate)
		}
	}
	fmt.Fprintln(out, "\nCapabilities")
	for i, candidate := range available {
		mark := " "
		if candidate.kind == capability.ExposureHTTP {
			mark = "x"
		}
		fmt.Fprintf(out, "  [%s] %d. %s\n", mark, i+1, candidate.label)
	}
	line, err := promptLine(reader, out, "Select capabilities (comma-separated numbers)", defaultCapabilitySelection(available))
	if err != nil {
		return nil, nil, profile, err
	}
	selectedIndexes, err := parseNumberSet(line, len(available))
	if err != nil {
		return nil, nil, profile, err
	}
	var kinds []capability.Kind
	for i, candidate := range available {
		if selectedIndexes[i+1] {
			kinds = append(kinds, candidate.kind)
		}
	}
	if len(kinds) == 0 {
		return nil, nil, profile, fmt.Errorf("select at least one application capability")
	}

	var secrets []string
	for _, kind := range kinds {
		if kind == capability.Secrets {
			value, err := promptLine(reader, out, "Required secret names (comma-separated)", "APP_SECRET")
			if err != nil {
				return nil, nil, profile, err
			}
			secrets = splitWizardItems(value)
		}
	}

	profile, err = guidedCapabilityPlacement(reader, out, profile, registry, kinds)
	if err != nil {
		return nil, nil, profile, err
	}
	return kinds, secrets, profile, nil
}

func guidedCapabilityPlacement(reader *bufio.Reader, out io.Writer, profile development.StackProfile, registry development.Registry, kinds []capability.Kind) (development.StackProfile, error) {
	for _, kind := range kinds {
		if profileHasCapabilityPreference(profile, kind) {
			continue
		}
		var compatible []development.Component
		for _, component := range profile.Components {
			adapter, err := registry.Resolve(component.Adapter)
			if err != nil {
				return profile, err
			}
			if adapter.Supports(capability.Requirement{Kind: kind}) {
				compatible = append(compatible, component)
			}
		}
		if len(compatible) == 0 {
			return profile, fmt.Errorf("stack %q cannot satisfy %s", profile.Metadata.Name, kind)
		}
		var targets []string
		if len(compatible) == 1 {
			targets = []string{compatible[0].ID}
		} else {
			fmt.Fprintf(out, "\n%s integration\n", kind)
			for i, component := range compatible {
				fmt.Fprintf(out, "  %d. %s (%s)\n", i+1, component.ID, strings.TrimPrefix(component.Adapter, "development/"))
			}
			line, err := promptLine(reader, out, "Components (comma-separated)", "1")
			if err != nil {
				return profile, err
			}
			set, err := parseNumberSet(line, len(compatible))
			if err != nil {
				return profile, err
			}
			for i, component := range compatible {
				if set[i+1] {
					targets = append(targets, component.ID)
				}
			}
			if len(targets) == 0 {
				return profile, fmt.Errorf("%s requires at least one component placement", kind)
			}
		}
		profile.Capabilities = append(profile.Capabilities, development.CapabilityPreference{Capability: kind, Components: targets})
	}
	sort.Slice(profile.Capabilities, func(i, j int) bool {
		return string(profile.Capabilities[i].Capability) < string(profile.Capabilities[j].Capability)
	})
	return profile, nil
}

func profileSupportsKind(profile development.StackProfile, registry development.Registry, kind capability.Kind) bool {
	for _, component := range profile.Components {
		adapter, err := registry.Resolve(component.Adapter)
		if err == nil && adapter.Supports(capability.Requirement{Kind: kind}) {
			return true
		}
	}
	return false
}

func profileHasCapabilityPreference(profile development.StackProfile, kind capability.Kind) bool {
	for _, preference := range profile.Capabilities {
		if preference.Capability == kind {
			return true
		}
	}
	return false
}

func printAppNewSummary(out io.Writer, root string, profile development.StackProfile, kinds []capability.Kind, files []string) {
	fmt.Fprintln(out, "\nSummary")
	fmt.Fprintf(out, "  Location      %s\n", displayUserPath(root))
	fmt.Fprintf(out, "  Stack         %s\n", profile.Metadata.Name)
	fmt.Fprintln(out, "  Components")
	for _, component := range profile.Components {
		fmt.Fprintf(out, "    %s (%s, %s)\n", component.ID, strings.TrimPrefix(component.Adapter, "development/"), component.Role)
	}
	fmt.Fprintln(out, "  Capabilities")
	for _, kind := range kinds {
		fmt.Fprintf(out, "    %s\n", kind)
	}
	fmt.Fprintf(out, "  Generated     %d files\n", len(files))
}

func promptNumber(reader *bufio.Reader, out io.Writer, label string, min, max, defaultValue int) (int, error) {
	value, err := promptLine(reader, out, label, strconv.Itoa(defaultValue))
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("invalid selection %q", value)
	}
	return n, nil
}

func splitWizardItems(value string) []string {
	var result []string
	for _, raw := range strings.Split(value, ",") {
		item := strings.TrimSpace(raw)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func parseNumberSet(value string, max int) (map[int]bool, error) {
	result := map[int]bool{}
	for _, raw := range splitWizardItems(value) {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > max {
			return nil, fmt.Errorf("invalid selection %q", raw)
		}
		result[n] = true
	}
	return result, nil
}

func defaultCapabilitySelection(available []greenfieldCapabilityChoice) string {
	for i, item := range available {
		if item.kind == capability.ExposureHTTP {
			return strconv.Itoa(i + 1)
		}
	}
	if len(available) > 0 {
		return "1"
	}
	return ""
}

func profileContainsPreference(profile development.StackProfile, candidate development.CapabilityPreference) bool {
	for _, preference := range profile.Capabilities {
		if preference.Capability != candidate.Capability ||
			preference.ImplementationPreference != candidate.ImplementationPreference ||
			preference.DevelopmentIntegration != candidate.DevelopmentIntegration {
			continue
		}
		left := append([]string(nil), preference.Components...)
		right := append([]string(nil), candidate.Components...)
		sort.Strings(left)
		sort.Strings(right)
		if strings.Join(left, ",") == strings.Join(right, ",") {
			return true
		}
	}
	return false
}
