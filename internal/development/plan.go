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

type DevelopmentPlan struct {
	SchemaVersion string              `json:"schema_version"`
	Application   string              `json:"application"`
	Profile       string              `json:"profile"`
	Actions       []Action            `json:"actions"`
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
		Profile:       profile.Name,
	}
	for _, component := range profile.Components {
		adapter, err := adapters.Resolve(component.Adapter)
		if err != nil {
			return DevelopmentPlan{}, fmt.Errorf("component %q: %w", component.ID, err)
		}
		actions, err := adapter.Plan(contract, profile, component)
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
