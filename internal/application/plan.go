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
	if err := m.Validate(); err != nil {
		return Plan{}, err
	}
	p := Plan{Application: m.Name, Environment: m.Environment}
	if HasManagedRuntimeServices(m) {
		p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "network", Description: fmt.Sprintf("ensure isolated network for %s-%s", m.Name, m.Environment)})
	}
	for _, instance := range PostgresInstanceNames(m) {
		resource := "postgres"
		if instance != defaultServiceInstance {
			resource += ":" + instance
		}
		p.Actions = append(p.Actions,
			Action{Kind: "ensure", Resource: resource + "-volume", Description: fmt.Sprintf("ensure dedicated PostgreSQL data volume for %s", instance)},
			Action{Kind: "ensure", Resource: resource, Description: fmt.Sprintf("ensure dedicated PostgreSQL service for %s", instance)},
		)
	}
	for _, instance := range RedisInstanceNames(m) {
		resource := "valkey"
		if instance != defaultServiceInstance {
			resource += ":" + instance
		}
		p.Actions = append(p.Actions,
			Action{Kind: "ensure", Resource: resource + "-volume", Description: fmt.Sprintf("ensure dedicated Valkey data volume for %s", instance)},
			Action{Kind: "ensure", Resource: resource, Description: fmt.Sprintf("ensure dedicated authenticated Valkey service for %s", instance)},
		)
	}
	if m.Services.Secrets {
		p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "secrets-scope", Description: "ensure isolated application secret scope"})
		for _, key := range RequiredSecretNames(m) {
			p.Actions = append(p.Actions, Action{Kind: "verify", Resource: "secret:" + key, Description: "require application secret before readiness"})
		}
	}
	if HasExplicitWorkload(m) {
		p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "workload", Description: "start and verify repository Compose workload"})
	}
	return p, nil
}
