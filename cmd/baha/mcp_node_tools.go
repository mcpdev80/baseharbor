package main

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

type machineNodeAddInput struct {
	NodeID      string `json:"node_id" jsonschema:"stable remote node identity"`
	TargetID    string `json:"target_id,omitempty" jsonschema:"remote Target name; defaults to node_id"`
	Runtime     string `json:"runtime" jsonschema:"remote runtime: docker or podman"`
	TenantID    string `json:"tenant_id" jsonschema:"canonical tenant UUID owned by the Core operator membership"`
	CoreURL     string `json:"core_url" jsonschema:"Core HTTPS operator API origin"`
	CoreAddress string `json:"core_address" jsonschema:"outbound Connector session endpoint in host:port form"`
	CAFile      string `json:"ca_file" jsonschema:"local public CA bundle authenticating the Core HTTPS endpoint"`
	Environment string `json:"environment,omitempty" jsonschema:"operator environment; defaults to dev"`
	Output      string `json:"output,omitempty" jsonschema:"optional local owner-only enrollment bundle path"`
}

type machineNodeConnectInput struct {
	EnrollmentFile string `json:"enrollment_file" jsonschema:"owner-only local enrollment bundle created by node.add"`
	ConnectorBin   string `json:"connector_bin,omitempty" jsonschema:"optional explicit baseharbor-node-connector executable"`
	Approval       bool   `json:"approval" jsonschema:"explicit approval to install and start the rootless connector service"`
}

type machineNodeTargetInput struct {
	Target string `json:"target" jsonschema:"configured remote connector Target"`
}

type machineNodeDisconnectInput struct {
	Target   string `json:"target" jsonschema:"configured remote connector Target"`
	Approval bool   `json:"approval" jsonschema:"explicit approval to revoke connector identity and delete the empty Target"`
}

func registerMCPNodeTools(server *mcp.Server) {
	mcp.AddTool(server, machineMCPTool("node.add", "Create a remote Target and protected one-use enrollment bundle. Token and nonce are written only to the owner-only bundle, never returned in the result.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineNodeAddInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "node.add", "", input.Environment, ""); err != nil {
			return machineMCPFailure(err)
		}
		args := []string{input.NodeID, "--runtime", input.Runtime, "--tenant-id", input.TenantID, "--core-url", input.CoreURL, "--core-address", input.CoreAddress, "--ca-file", input.CAFile}
		if input.TargetID != "" {
			args = append(args, "--target", input.TargetID)
		}
		if input.Environment != "" {
			args = append(args, "--environment", input.Environment)
		}
		if input.Output != "" {
			args = append(args, "--output", input.Output)
		}
		args = append(args, "--json")
		var out bytes.Buffer
		if err := nodeAdd(ctx, args, &out, &out); err != nil {
			return machineMCPFailure(err)
		}
		var result nodeAddResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			return machineMCPFailure(machine.Wrap(machine.ErrorInternal, err, "Inspect node enrollment output and retry.", false))
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("node.connect", "Consume one protected local enrollment bundle and install/start the rootless Node Connector service.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineNodeConnectInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "node.connect", "", "", ""); err != nil {
			return machineMCPFailure(err)
		}
		if !input.Approval {
			return machineMCPFailure(machine.NewError(machine.ErrorApprovalRequired, "node.connect requires explicit approval", "Set approval=true after reviewing the enrollment file and host.", false))
		}
		args := []string{input.EnrollmentFile}
		if input.ConnectorBin != "" {
			args = append(args, "--connector-bin", input.ConnectorBin)
		}
		args = append(args, "--json")
		var out bytes.Buffer
		if err := nodeConnect(ctx, args, &out, &out); err != nil {
			return machineMCPFailure(err)
		}
		var result nodeConnectResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			return machineMCPFailure(machine.Wrap(machine.ErrorInternal, err, "Inspect node connector output and retry.", false))
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("node.list", "List configured remote connector nodes without credentials.", false), func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "node.list", "", "", ""); err != nil {
			return machineMCPFailure(err)
		}
		var out bytes.Buffer
		if err := nodeList(ctx, []string{"--json"}, &out, &out); err != nil {
			return machineMCPFailure(err)
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			return machineMCPFailure(machine.Wrap(machine.ErrorInternal, err, "Inspect node list output and retry.", false))
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("node.status", "Inspect Core enrollment state for one configured remote node.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineNodeTargetInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "node.status", input.Target, "", ""); err != nil {
			return machineMCPFailure(err)
		}
		var out bytes.Buffer
		if err := nodeStatus(ctx, []string{input.Target, "--json"}, &out, &out); err != nil {
			return machineMCPFailure(err)
		}
		var result nodeStatusResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			return machineMCPFailure(machine.Wrap(machine.ErrorInternal, err, "Inspect node status output and retry.", false))
		}
		return nil, result, nil
	})

	mcp.AddTool(server, machineMCPTool("node.disconnect", "Revoke a node identity first, then remove its empty Target registration.", false), func(ctx context.Context, req *mcp.CallToolRequest, input machineNodeDisconnectInput) (*mcp.CallToolResult, any, error) {
		if err := authorizeCurrentMCPContext(ctx, "node.disconnect", input.Target, "", ""); err != nil {
			return machineMCPFailure(err)
		}
		if !input.Approval {
			return machineMCPFailure(machine.NewError(machine.ErrorApprovalRequired, "node.disconnect requires explicit approval", "Set approval=true after reviewing the remote Target.", false))
		}
		var out bytes.Buffer
		if err := nodeDisconnect(ctx, []string{input.Target, "--yes", "--json"}, &out, &out); err != nil {
			return machineMCPFailure(err)
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			return machineMCPFailure(machine.Wrap(machine.ErrorInternal, err, "Inspect node disconnect output and retry.", false))
		}
		return nil, result, nil
	})
}
