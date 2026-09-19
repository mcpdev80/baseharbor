package application

import "fmt"

type Action struct {
	Kind        string
	Resource    string
	Description string
}

type Plan struct {
	Application string
	Environment string
	Actions     []Action
}

func BuildPlan(m Manifest) (Plan, error) {
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		return Plan{}, err
	}

	if _, err := ResolveCapabilityResources(contract); err != nil {
		return Plan{}, err
	}

	p := Plan{Application: m.Name, Environment: m.Environment}
	if HasManagedRuntimeServices(m) {
		p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "network", Description: fmt.Sprintf("ensure isolated network for %s-%s", m.Name, m.Environment)})
	}
	for _, capability := range contract.Capabilities {
		switch capability.Kind {
		case CapabilitySQL:
			resource := "postgres"
			if capability.Name != defaultServiceInstance {
				resource += ":" + capability.Name
			}
			p.Actions = append(p.Actions,
				Action{Kind: "ensure", Resource: resource + "-volume", Description: fmt.Sprintf("ensure dedicated PostgreSQL data volume for %s", capability.Name)},
				Action{Kind: "ensure", Resource: resource, Description: fmt.Sprintf("ensure dedicated PostgreSQL service for %s", capability.Name)},
			)
		case CapabilityKeyValue:
			resource := "valkey"
			if capability.Name != defaultServiceInstance {
				resource += ":" + capability.Name
			}
			p.Actions = append(p.Actions,
				Action{Kind: "ensure", Resource: resource + "-volume", Description: fmt.Sprintf("ensure dedicated Valkey data volume for %s", capability.Name)},
				Action{Kind: "ensure", Resource: resource, Description: fmt.Sprintf("ensure dedicated authenticated Valkey service for %s", capability.Name)},
			)
		case CapabilityExposureHTTP:
			p.Actions = append(p.Actions, Action{
				Kind:        "ensure",
				Resource:    "exposure:" + capability.Name,
				Description: fmt.Sprintf("ensure managed HTTP exposure %s", capability.Name),
			})
		default:
			return Plan{}, fmt.Errorf("unsupported application capability %q", capability.Kind)
		}
	}
	if contract.Secrets.Managed {
		p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "secrets-scope", Description: "ensure isolated application secret scope"})
		for _, requirement := range contract.Secrets.Required {
			p.Actions = append(p.Actions, Action{Kind: "verify", Resource: "secret:" + requirement.Name, Description: "require application secret before readiness"})
		}
	}
	if HasExplicitWorkload(m) {
		p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "workload", Description: "start and verify repository Compose workload"})
	}
	return p, nil
}
