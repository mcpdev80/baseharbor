package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatApplicationOverviewReady(t *testing.T) {
	overview := applicationOverview{
		Name:         "mailflow",
		Environment:  "production",
		ManifestPath: "/work/mailflow/baseharbor.yaml",
		Ready:        true,
		Postgres: []overviewResource{
			{Name: "primary", State: "healthy"},
		},
		Valkey: []overviewResource{
			{Name: "cache", State: "healthy"},
		},
		Workload: repositoryWorkloadStatus{
			Found: true,
			Services: []workloadServiceStatus{
				{Service: "api", State: "running", Health: "healthy", Ready: true},
				{Service: "web", State: "running", Ready: true},
			},
		},
		SecretsDeclared: true,
		SecretsRequired: 3,
		SecretsReady:    3,
		SecretsState:    "healthy",
		BrokerState:     "healthy",
	}

	var out bytes.Buffer
	formatApplicationOverview(&out, overview)
	text := out.String()
	for _, wanted := range []string{
		"Application: mailflow",
		"Environment: production",
		"Status: READY",
		"PostgreSQL primary",
		"Valkey     cache",
		"api                  running health=healthy",
		"web                  running",
		"required             3",
		"ready                3",
		"runtime broker       healthy",
		"Last backup",
		"not recorded yet",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("overview output missing %q:\n%s", wanted, text)
		}
	}
}

func TestFormatApplicationOverviewHidesSecretValues(t *testing.T) {
	overview := applicationOverview{
		Name:            "mailflow",
		Environment:     "production",
		SecretsDeclared: true,
		SecretsRequired: 1,
		SecretsReady:    0,
		SecretsState:    "not ready",
		BrokerState:     "not ready",
	}

	var out bytes.Buffer
	formatApplicationOverview(&out, overview)
	text := out.String()
	if strings.Contains(text, "SECRET_KEY") || strings.Contains(text, "postgres://") || strings.Contains(text, "redis://") {
		t.Fatalf("overview exposed secret-bearing detail:\n%s", text)
	}
	if !strings.Contains(text, "Status: NOT READY") {
		t.Fatalf("overview should report NOT READY:\n%s", text)
	}
}
