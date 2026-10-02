package main

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationlifecycle"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
)

func registerMCPProviderOrganizationTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("provider.add", "Register an externally owned provider from endpoint plus secret-safe credential/trust references.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineProviderAddInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "provider.add", "", "", ""); err != nil {
			return machineMCPFailure(err)
		}
		reg, err := providerExternalRegistration(providerExternalArgs{
			ID:                input.ID,
			ProviderID:        input.ProviderID,
			ProviderVersion:   input.ProviderVersion,
			ProviderProtocol:  input.ProviderProtocol,
			Kind:              input.Kind,
			Capabilities:      append([]string(nil), input.Capabilities...),
			Endpoint:          input.Endpoint,
			CredentialRef:     input.CredentialRef,
			TrustMode:         input.TrustMode,
			CAReference:       input.CAReference,
			ClientCertificate: input.ClientCertificate,
			ClientKey:         input.ClientKey,
			Directory:         input.CertificateDir,
		})
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := application.RegisterExternalProvider(reg); err != nil {
			return machineMCPFailure(err)
		}
		return nil, reg.Public(), nil
	})

	mcp.AddTool(server, machineMCPTool("provider.remove", "Remove BaseHarbor registration for an externally owned provider. Foreign infrastructure is never mutated.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineProviderRemoveInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "provider.remove", "", "", ""); err != nil {
			return machineMCPFailure(err)
		}
		if err := applicationlifecycle.RequireApproval("provider.remove", input.Approval); err != nil {
			return machineMCPFailure(err)
		}
		id := strings.TrimSpace(input.ID)
		item, err := application.InspectExternalProvider(id)
		if err != nil {
			return machineMCPFailure(err)
		}
		if err := application.RemoveExternalProvider(id); err != nil {
			return machineMCPFailure(err)
		}
		result := struct {
			ID             string `json:"id"`
			Removed        bool   `json:"removed"`
			ForeignMutated bool   `json:"foreign_mutated"`
		}{ID: item.ID, Removed: true, ForeignMutated: false}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("organization.set", "Resolve and activate one organization/platform configuration source. The immutable digest/revision and provenance are persisted explicitly.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineOrganizationSetInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeMCPOperation(ctx, "organization.set", "", input.Environment, "", ""); err != nil {
			return machineMCPFailure(err)
		}
		source := orgconfig.Source{Kind: orgconfig.SourceKind(strings.ToLower(strings.TrimSpace(input.Source))), Location: strings.TrimSpace(input.Location), Requested: strings.TrimSpace(input.Requested)}
		state, err := orgconfig.Activate(ctx, source)
		if err != nil {
			return machineMCPFailure(err)
		}
		effective, err := orgconfig.ResolveEffective(state, input.Environment)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, organizationView{ContractVersion: orgconfig.ContractVersion, State: state, Effective: effective}, nil
	})

	mcp.AddTool(server, machineMCPTool("organization.update", "Explicitly activate the configured organization source at its newly resolved immutable version after prior review.", true), func(ctx context.Context, req *mcp.CallToolRequest, input machineOrganizationUpdateInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeMCPOperation(ctx, "organization.update", "", input.Environment, "", ""); err != nil {
			return machineMCPFailure(err)
		}
		if err := applicationlifecycle.RequireApproval("organization.update", input.Approval); err != nil {
			return machineMCPFailure(err)
		}
		state, err := orgconfig.Refresh(ctx)
		if err != nil {
			return machineMCPFailure(err)
		}
		effective, err := orgconfig.ResolveEffective(state, input.Environment)
		if err != nil {
			return machineMCPFailure(err)
		}
		return nil, organizationView{ContractVersion: orgconfig.ContractVersion, State: state, Effective: effective}, nil
	})
}
