package development

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

const DevelopmentPlanVersion = "baseharbor.development-plan/v1"

type ActionKind string

const (
	ActionDependency ActionKind = "dependency"
	ActionBinding    ActionKind = "binding"
	ActionSource     ActionKind = "source"
	ActionBuild      ActionKind = "build"
	ActionHealth     ActionKind = "health"
	ActionTelemetry  ActionKind = "telemetry"
)

type Action struct {
	Kind       ActionKind      `json:"kind"`
	Component  string          `json:"component"`
	Capability capability.Kind `json:"capability,omitempty"`
	Name       string          `json:"name"`
	Value      string          `json:"value,omitempty"`
}

type PlannedSource struct {
	Component string     `json:"component"`
	Source    string     `json:"source"`
	Type      SourceKind `json:"type"`
	Identity  string     `json:"identity"`
	Ref       string     `json:"ref,omitempty"`
	SubPath   string     `json:"sub_path,omitempty"`
	Image     string     `json:"image,omitempty"`
}

type DevelopmentPlan struct {
	SchemaVersion string          `json:"schema_version"`
	Application   string          `json:"application"`
	Profile       string          `json:"profile"`
	Sources       []PlannedSource `json:"sources,omitempty"`
	Actions       []Action        `json:"actions"`
}

func BuildPlan(contract application.PortableContract, profile StackProfile, adapters Registry) (DevelopmentPlan, error) {
	if strings.TrimSpace(contract.Application) == "" {
		return DevelopmentPlan{}, fmt.Errorf("development plan requires application contract")
	}
	if err := profile.Validate(); err != nil {
		return DevelopmentPlan{}, err
	}
	plan := DevelopmentPlan{
		SchemaVersion: DevelopmentPlanVersion,
		Application:   contract.Application,
		Profile:       profile.Metadata.Name,
	}
	for _, component := range profile.Components {
		adapter, err := adapters.Resolve(component.Adapter)
		if err != nil {
			return DevelopmentPlan{}, fmt.Errorf("component %q: %w", component.ID, err)
		}
		componentContract := contractForComponent(contract, profile, component.ID)
		actions, err := adapter.Plan(componentContract, profile, component)
		if err != nil {
			return DevelopmentPlan{}, fmt.Errorf("component %q adapter %q: %w", component.ID, component.Adapter, err)
		}
		for i := range actions {
			if strings.TrimSpace(actions[i].Component) == "" {
				actions[i].Component = component.ID
			}
			if actions[i].Component != component.ID {
				return DevelopmentPlan{}, fmt.Errorf("adapter %q returned action for foreign component %q", component.Adapter, actions[i].Component)
			}
		}
		plan.Actions = append(plan.Actions, actions...)
	}
	sort.SliceStable(plan.Actions, func(i, j int) bool {
		if plan.Actions[i].Component != plan.Actions[j].Component {
			return plan.Actions[i].Component < plan.Actions[j].Component
		}
		if plan.Actions[i].Kind != plan.Actions[j].Kind {
			return plan.Actions[i].Kind < plan.Actions[j].Kind
		}
		if plan.Actions[i].Capability != plan.Actions[j].Capability {
			return plan.Actions[i].Capability < plan.Actions[j].Capability
		}
		if plan.Actions[i].Name != plan.Actions[j].Name {
			return plan.Actions[i].Name < plan.Actions[j].Name
		}
		return plan.Actions[i].Value < plan.Actions[j].Value
	})
	return plan, nil
}

func contractForComponent(contract application.PortableContract, profile StackProfile, componentID string) application.PortableContract {
	filtered := contract
	filtered.Capabilities = make([]application.CapabilityRequirement, 0, len(contract.Capabilities))
	for _, requirement := range contract.Capabilities {
		if profileCapabilityAppliesToComponent(profile, requirement.Kind, componentID) {
			filtered.Capabilities = append(filtered.Capabilities, requirement)
		}
	}
	if !profileCapabilityAppliesToComponent(profile, capability.Secrets, componentID) {
		filtered.Secrets.Managed = false
		filtered.Secrets.Required = nil
	}
	return filtered
}

func profileCapabilityAppliesToComponent(profile StackProfile, kind capability.Kind, componentID string) bool {
	var matched bool
	for _, preference := range profile.Capabilities {
		if preference.Capability != kind {
			continue
		}
		matched = true
		for _, candidate := range preference.Components {
			if candidate == componentID {
				return true
			}
		}
	}
	return !matched
}


func BuildPlanForWorkspace(contract application.PortableContract, profile StackProfile, adapters Registry, workspace WorkspaceResolution) (DevelopmentPlan, error) {
	plan, err := BuildPlan(contract, profile, adapters)
	if err != nil {
		return DevelopmentPlan{}, err
	}
	if workspace.Application != contract.Application {
		return DevelopmentPlan{}, fmt.Errorf("workspace application %q does not match application contract %q", workspace.Application, contract.Application)
	}
	resolved := make(map[string]ResolvedComponentSource, len(workspace.Components))
	for _, source := range workspace.Components {
		resolved[source.Component] = source
	}
	for _, component := range profile.Components {
		source, ok := resolved[component.ID]
		if !ok {
			return DevelopmentPlan{}, fmt.Errorf("development component %q has no resolved source identity", component.ID)
		}
		plan.Sources = append(plan.Sources, PlannedSource{
			Component: component.ID,
			Source:    source.Source,
			Type:      source.Type,
			Identity:  source.Identity,
			Ref:       source.Ref,
			SubPath:   source.SubPath,
			Image:     source.Image,
		})
	}
	sort.Slice(plan.Sources, func(i, j int) bool { return plan.Sources[i].Component < plan.Sources[j].Component })
	return plan, nil
}

func ValidateWorkspace(contract application.PortableContract, profile StackProfile, adapters Registry, workspace WorkspaceResolution) (map[string]Validation, error) {
	resolved := make(map[string]ResolvedComponentSource, len(workspace.Components))
	for _, source := range workspace.Components {
		resolved[source.Component] = source
	}
	results := make(map[string]Validation, len(profile.Components))
	for _, component := range profile.Components {
		source, ok := resolved[component.ID]
		if !ok {
			return nil, fmt.Errorf("development component %q has no resolved source identity", component.ID)
		}
		if source.Type != SourceRepository {
			return nil, fmt.Errorf("development component %q uses %s source %q; source-backed adapter validation requires a local repository worktree", component.ID, source.Type, source.Identity)
		}
		adapter, err := adapters.Resolve(component.Adapter)
		if err != nil {
			return nil, err
		}
		validation, err := adapter.Validate(source.Root, contractForComponent(contract, profile, component.ID), component)
		if err != nil {
			return nil, fmt.Errorf("validate component %q at resolved source %s: %w", component.ID, source.Root, err)
		}
		results[component.ID] = validation
	}
	return results, nil
}
