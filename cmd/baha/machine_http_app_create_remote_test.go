package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/development"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestHTTPAppCreateRejectsForeignAuthorityBeforeMutation(t *testing.T) {
	authorized := machine.OperationContext{
		Target: "remote-docker", Application: "demo", Environment: "dev",
	}
	for _, tc := range []struct {
		name   string
		target string
		app    string
		env    string
	}{
		{"foreign-target", "other-docker", "demo", "dev"},
		{"foreign-application", "remote-docker", "other-app", "dev"},
		{"foreign-environment", "remote-docker", "demo", "prod"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(machineApplicationCreateInput{
				Target: tc.target,
				Intent: application.Manifest{Name: tc.app, Environment: tc.env},
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = executeHTTPAppCreate(context.Background(), authorized, raw)
			if err == nil || machine.Classify(err).Code != machine.ErrorPolicyDenied {
				t.Fatalf("foreign remote creation selector was not denied: %v", err)
			}
		})
	}
}

func TestHTTPAppCreateRequiresCompleteAuthorizedScope(t *testing.T) {
	input, err := json.Marshal(machineApplicationCreateInput{
		Intent: application.Manifest{Name: "demo", Environment: "dev"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []machine.OperationContext{
		{Application: "demo", Environment: "dev"},
		{Target: "remote-docker", Environment: "dev"},
		{Target: "remote-docker", Application: "demo"},
	} {
		_, err := executeHTTPAppCreate(context.Background(), scope, input)
		if err == nil || machine.Classify(err).Code != machine.ErrorValidationFailed {
			t.Fatalf("incomplete remote app creation authority was accepted: %+v: %v", scope, err)
		}
	}
}

func TestHTTPAppCreateIsDiscoverable(t *testing.T) {
	for _, id := range (&bahaMachineExecutor{}).SupportedOperationIDs() {
		if id == "app.create" {
			return
		}
	}
	t.Fatal("authorized app.create missing from Machine HTTP discovery")
}

func TestHTTPAppCreateValidatesSourceModelBeforeRegistration(t *testing.T) {
	input, err := json.Marshal(machineApplicationCreateInput{
		Target: "remote-docker",
		Intent: application.Manifest{Name: "demo", Environment: "dev"},
		SourceModel: &development.SourceModel{
			SchemaVersion: development.SourceModelVersion,
			Application: "demo",
			Sources: []development.SourceDefinition{{
				ID: "app", Type: development.SourceRepository,
				Repository: "https://git.example/demo.git", Ref: "main",
			}},
			Components: []development.ComponentSource{{Component: "app", Source: "app"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = executeHTTPAppCreate(context.Background(), machine.OperationContext{
		Target: "remote-docker", Application: "demo", Environment: "dev",
	}, input)
	if err == nil || machine.Classify(err).Code != machine.ErrorValidationFailed {
		t.Fatalf("mutable remote source reached registration instead of being rejected: %v", err)
	}
}
