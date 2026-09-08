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
	p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "network", Description: fmt.Sprintf("ensure isolated network for %s-%s", m.Name, m.Environment)})
	if m.Services.Postgres {
		p.Actions = append(p.Actions,
			Action{Kind: "ensure", Resource: "postgres-volume", Description: "ensure dedicated PostgreSQL data volume"},
			Action{Kind: "ensure", Resource: "postgres", Description: "ensure dedicated PostgreSQL service"},
		)
	}
	if m.Services.Redis {
		p.Actions = append(p.Actions,
			Action{Kind: "ensure", Resource: "valkey-volume", Description: "ensure dedicated Valkey data volume"},
			Action{Kind: "ensure", Resource: "valkey", Description: "ensure dedicated authenticated Valkey service"},
		)
	}
	if m.Services.Secrets {
		p.Actions = append(p.Actions, Action{Kind: "ensure", Resource: "secrets-scope", Description: "ensure isolated application secret scope"})
		for _, key := range RequiredSecretNames(m) {
			p.Actions = append(p.Actions, Action{Kind: "verify", Resource: "secret:" + key, Description: "require application secret before readiness"})
		}
	}
	return p, nil
}
