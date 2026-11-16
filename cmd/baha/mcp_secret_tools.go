package main

import (
	"context"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
)

type machineSecretInput struct {
	Target      string `json:"target,omitempty"`
	Application string `json:"application,omitempty"`
	Environment string `json:"environment,omitempty"`
}
type machineSecretSetInput struct {
	machineSecretInput
	Key  string `json:"key"`
	File string `json:"file" jsonschema:"owner-only regular file on the execution host; no secret value in tool arguments"`
}
type machineSecretDeleteInput struct {
	machineSecretInput
	Key      string `json:"key"`
	Approval bool   `json:"approval"`
}
type machineTLSMaterialInput struct {
	machineSecretInput
	CertificateFile string `json:"certificate_file"`
	PrivateKeyFile  string `json:"private_key_file" jsonschema:"owner-only regular private key file on the execution host"`
	ChainFile       string `json:"chain_file,omitempty"`
}

func resolveMachineSecretApplication(ctx context.Context, store application.Store, input machineSecretInput) (resolvedApplication, error) {
	return resolveApplication(withTargetOverride(ctx, input.Target), store, machineApplicationArgs(input.Application, input.Environment), "secret")
}
func readProtectedSecretInput(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, machine.Wrap(machine.ErrorValidationFailed, errors.New("protected input is unavailable"), "Provide a readable owner-only regular file on the execution host.", false)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, machine.Wrap(machine.ErrorValidationFailed, errors.New("protected input must be an owner-only regular file"), "Use a regular file with mode 0600 or stricter.", false)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("protected input is unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0077 != 0 {
		return nil, errors.New("protected input changed while opening")
	}
	return readSecretValue(file)
}
func registerMCPSecretTools(server *mcp.Server, store application.Store) {
	mcp.AddTool(server, machineMCPTool("secret.list", "List configured secret names without exposing values.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineSecretInput) (*mcp.CallToolResult, any, error) {
		resolved, err := resolveMachineSecretApplication(ctx, store, input)
		if err != nil {
			return machineMCPFailure(err)
		}
		items, err := listApplicationSecrets(ctx, resolved)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, map[string]any{"application": resolved.Manifest.Name, "secrets": items}, nil
	})
	mcp.AddTool(server, machineMCPTool("secret.set", "Store a secret from protected host input after shared operator authorization.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineSecretSetInput) (*mcp.CallToolResult, any, error) {
		resolved, err := resolveMachineSecretApplication(ctx, store, input.machineSecretInput)
		if err != nil {
			return machineMCPFailure(err)
		}
		result, err := setApplicationSecret(ctx, resolved, input.Key, func() ([]byte, error) { return readProtectedSecretInput(input.File) })
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("secret.delete", "Permanently delete one secret and its managed versions after explicit approval.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineSecretDeleteInput) (*mcp.CallToolResult, any, error) {
		resolved, err := resolveMachineSecretApplication(ctx, store, input.machineSecretInput)
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := authorizeApplicationOperation(ctx, "secret.delete", resolved); err != nil {
			return machineMCPFailure(err)
		}
		if err := applicationlifecycle.RequireApproval("secret.delete", input.Approval); err != nil {
			return machineMCPFailure(err)
		}
		result, err := deleteApplicationSecret(ctx, resolved, input.Key, true)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, result, nil
	})
	mcp.AddTool(server, machineMCPTool("secret.tls-set", "Validate and store certificate/private-key material from protected host references.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineTLSMaterialInput) (*mcp.CallToolResult, any, error) {
		resolved, err := resolveMachineSecretApplication(ctx, store, input.machineSecretInput)
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := authorizeApplicationOperation(ctx, "secret.tls-set", resolved); err != nil {
			return machineMCPFailure(err)
		}
		if err := setApplicationTLSMaterialWithKeyReader(ctx, resolved, input.CertificateFile, input.PrivateKeyFile, input.ChainFile, readProtectedSecretInput); err != nil {
			return machineMCPFailure(err)
		}

		return nil, secretMutationResult{Application: resolved.Manifest.Name, Environment: resolved.Manifest.Environment, Key: "TLS_CERT_FILE/TLS_KEY_FILE", Updated: true}, nil
	})
}
