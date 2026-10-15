package main

import (
	"context"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Project the existing Manifest types rather than duplicate a portable parser
// or contract. Only authoring omits identity; canonical outputs retain it.
func machineApplicationAuthoringTool[Input any](operation, description string) *mcp.Tool {
	intent, err := jsonschema.For[application.Manifest](nil)
	if err != nil {
		panic(err)
	}
	delete(intent.Properties, "ApplicationID")
	intent.Required = []string{"Version", "Name", "Environment"}
	intent.Description = "Declare application needs. BaseHarbor assigns and persists the application ID; do not supply ApplicationID."
	schema, err := jsonschema.For[Input](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[application.Manifest](): intent}})
	if err != nil {
		panic(err)
	}
	tool := machineMCPTool(operation, description, false)
	tool.InputSchema = schema
	return tool
}

func createMachineApplication(ctx context.Context, intent application.Manifest) (applicationAdoptionResult, error) {
	if intent.ApplicationID != "" {
		return applicationAdoptionResult{}, machine.NewError(machine.ErrorValidationFailed, "ApplicationID is system-owned.", "Omit ApplicationID; BaseHarbor assigns the identity during creation.", false)
	}
	if err := intent.Validate(); err != nil {
		return applicationAdoptionResult{}, err
	}
	if err := authorizeMCPOperation(ctx, "app.create", "", intent.Environment, "", ""); err != nil {
		return applicationAdoptionResult{}, err
	}
	var err error
	intent.ApplicationID, err = application.NewApplicationID()
	if err != nil {
		return applicationAdoptionResult{}, err
	}
	path, err := createTargetManagedApplication(ctx, intent)
	if err != nil {
		return applicationAdoptionResult{}, err
	}
	return applicationAdoptionResult{Application: intent.Name, ApplicationID: intent.ApplicationID, Environment: intent.Environment, Manifest: path}, nil
}
