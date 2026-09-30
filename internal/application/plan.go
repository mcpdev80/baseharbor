package application

import "fmt"

type Action struct {
	Kind        string `json:"kind"`
	Resource    string `json:"resource"`
	Description string `json:"description"`
}

type Plan struct {
	ContractVersion string   `json:"contract_version"`
	Application     string   `json:"application"`
	Environment     string   `json:"environment"`
	Actions         []Action `json:"actions"`
}

func BuildPlan(m Manifest) (Plan, error) {
	contract, err := PortableContractFromManifest(m)
	if err != nil {
		return Plan{}, err
	}

	if _, err := ResolveCapabilityResources(contract); err != nil {
		return Plan{}, err
	}

	p := Plan{ContractVersion: "v1", Application: m.Name, Environment: m.Environment}
	for _, capability := range contract.Capabilities {
		switch capability.Kind {
		case CapabilitySQL:
			resource := "database.sql:" + capability.Name
			p.Actions = append(p.Actions,
				Action{Kind: "ensure", Resource: resource, Description: fmt.Sprintf("ensure SQL capability resource %s", capability.Name)},
				Action{Kind: "verify", Resource: resource, Description: fmt.Sprintf("verify application-facing SQL access for %s", capability.Name)},
			)
		case CapabilityKeyValue:
			resource := "cache.key-value:" + capability.Name
			p.Actions = append(p.Actions,
				Action{Kind: "ensure", Resource: resource, Description: fmt.Sprintf("ensure key-value cache capability resource %s", capability.Name)},
				Action{Kind: "verify", Resource: resource, Description: fmt.Sprintf("verify application-facing key-value cache access for %s", capability.Name)},
			)
		case CapabilityDurableKeyValue:
			resource := "database.key-value:" + capability.Name
			p.Actions = append(p.Actions,
				Action{Kind: "ensure", Resource: resource, Description: fmt.Sprintf("ensure durable key-value database capability resource %s", capability.Name)},
				Action{Kind: "verify", Resource: resource, Description: fmt.Sprintf("verify durable application-facing key-value write/read access for %s", capability.Name)},
			)
		case CapabilityObjectStorageS3:
			p.Actions = append(p.Actions,
				Action{Kind: "ensure", Resource: "s3-bucket:" + capability.Name, Description: fmt.Sprintf("ensure isolated S3 bucket %s", capability.Name)},
				Action{Kind: "verify", Resource: "s3-bucket:" + capability.Name, Description: "verify authenticated S3 Put/Get flow"},
			)
		case CapabilityExposureHTTP:
			p.Actions = append(p.Actions, Action{
				Kind:        "ensure",
				Resource:    "exposure:" + capability.Name,
				Description: fmt.Sprintf("ensure managed HTTP exposure %s", capability.Name),
			})
		case CapabilityTelemetryOTLP:
			p.Actions = append(p.Actions, Action{
				Kind:        "ensure",
				Resource:    "telemetry.otlp:" + capability.Name,
				Description: "ensure OTLP export binding",
			})
		case CapabilityMetrics:
			p.Actions = append(p.Actions, Action{
				Kind:        "ensure",
				Resource:    "metrics:" + capability.Name,
				Description: fmt.Sprintf("ensure metrics source %s", capability.Name),
			})
		case CapabilityLogs:
			p.Actions = append(p.Actions, Action{
				Kind:        "ensure",
				Resource:    "logs:" + capability.Name,
				Description: fmt.Sprintf("ensure workload log collection for %s", capability.Name),
			})
		case CapabilityIdentity:
			p.Actions = append(p.Actions,
				Action{
					Kind:        "ensure",
					Resource:    "identity:" + capability.Name,
					Description: "ensure isolated OIDC application identity",
				},
				Action{
					Kind:        "verify",
					Resource:    "identity:" + capability.Name,
					Description: "verify OIDC discovery, client binding and authentication policy",
				},
			)
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
		p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "workload", Description: "start and verify repository workload"})
	}
	return p, nil
}
