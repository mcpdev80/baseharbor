package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
)

type greenfieldCapabilityChoice struct {
	kind  capability.Kind
	label string
}

type guidedStackSelection struct {
	RawProfile      *development.StackProfile
	Effective       development.StackProfile
	SaveScope       development.ProfileScope
	Created         bool
}

func runAppNewWizard(ctx context.Context, out, errOut io.Writer) error {
	reader := bufio.NewReader(appNewInput)
	registry, err := referenceDevelopmentRegistry()
	if err != nil {
		return err
	}
	catalog, err := development.LoadProfileCatalog(".", builtinDevelopmentProfiles(registry))
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Create a new application")
	fmt.Fprintln(out)

	name, err := promptLine(reader, out, "Application name", "")
	if err != nil {
		return err
	}
	name = slugifyAppName(name)
	if name == "" {
		return usageError("application name is required", "Enter a lowercase application name such as catalog-api.")
	}

	root, err := guidedNewApplicationRoot(reader, out, name)
	if err != nil {
		return err
	}

	stack, err := guidedStackProfile(reader, out, catalog, registry)
	if err != nil {
		return err
	}

	kinds, secrets, profile, err := guidedGreenfieldCapabilities(reader, out, stack.Effective, registry)
	if err != nil {
		return err
	}

	request := development.NewApplicationRequest{
		Name:         name,
		Environment:  "dev",
		Profile:      &profile,
		Capabilities: kinds,
		Secrets:      secrets,
	}
	preview, err := development.BootstrapApplication(request, registry)
	if err != nil {
		return err
	}

	printAppNewSummary(out, root, profile, kinds, preview.FilePaths)
	confirm, err := promptYesNo(reader, out, "Create application?", true)
	if err != nil {
		return err
	}
	if !confirm {
		fmt.Fprintln(out, "No changes were made.")
		return nil
	}

	result, err := development.CreateApplication(root, request, registry)
	if err != nil {
		return err
	}
	if stack.Created && stack.RawProfile != nil {
		saveRoot := root
		if _, err := development.SaveProfile(*stack.RawProfile, stack.SaveScope, saveRoot); err != nil {
			return fmt.Errorf("application was created but reusable stack profile could not be saved: %w", err)
		}
	}

	fmt.Fprintf(out, "Created %s\n", result.Manifest.Name)
	fmt.Fprintf(out, "Location: %s\n", displayUserPath(root))
	fmt.Fprintf(out, "Stack: %s\n", profile.Metadata.Name)
	fmt.Fprintln(out, "Validation: SATISFIED")
	fmt.Fprintln(out, "Next: cd "+displayUserPath(root)+" && baha up")
	return nil
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
