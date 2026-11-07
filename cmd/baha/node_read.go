package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// Read-only node inspection and authoritative Core lifecycle status checks.
// Keep enrollment/connection mutations in node.go.
func nodeList(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "node list")
	if err != nil {
		return err
	}
	if len(filtered) != 0 {
		return usageError("baha node list does not accept positional arguments", "Use --json for structured output.")
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	names := cfg.TargetNames()
	items := make([]nodeListItem, 0)
	for _, name := range names {
		target := cfg.Targets[name]
		access, ok := cfg.Access[target.Access.Reference]
		if !ok || access.Provider != nodeConnectorProvider {
			continue
		}
		item := nodeListItem{TargetID: name, NodeID: access.Reference, TenantID: target.TenantID, Runtime: target.Runtime.Provider, State: "PENDING"}
		// Trust state comes only from the authoritative Core lifecycle status
		// (certificate issuance, expiry and revocation). A node whose authority
		// cannot be read stays PENDING with an explicit reason; it is never
		// labelled from a local grant record or from session expiry.
		if record, recordErr := loadNodeRecord(name); recordErr == nil {
			status, statusErr := requestNodeLifecycle(ctx, record, targetenrollment.StatusPath)
			if statusErr == nil {
				item.State = classifyNodeTrust(status.Revoked, status.Enrolled, status.ExpiresAt, time.Now())
				item.ExpiresAt = status.ExpiresAt
			} else {
				item.unconfirmed = statusErr.Error()
			}
		} else {
			item.unconfirmed = recordErr.Error()
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].TargetID < items[j].TargetID })
	if format == outputJSON {
		return writeJSON(out, map[string]any{"contract_version": machine.ContractVersion, "nodes": items})
	}
	if len(items) == 0 {
		fmt.Fprintln(out, "No remote connector nodes configured.")
		return nil
	}
	for _, item := range items {
		fmt.Fprintf(out, "%-24s %-24s %-8s %-10s\n", item.TargetID, item.NodeID, item.Runtime, item.State)
		if !item.ExpiresAt.IsZero() {
			fmt.Fprintf(out, "%-24s %-24s %-8s Expires %s\n", "", "", "", item.ExpiresAt.UTC().Format(time.RFC3339))
		}
		if item.State == "PENDING" && item.unconfirmed != "" {
			fmt.Fprintf(out, "%-24s %-24s %-8s Authority %s\n", "", "", "", item.unconfirmed)
		}
	}
	return nil
}

func nodeStatus(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "node status")
	if err != nil {
		return err
	}
	if len(filtered) != 1 {
		return usageError("baha node status requires TARGET", "Run 'baha node list' to choose a configured remote Target.")
	}
	record, err := loadNodeRecord(filtered[0])
	if err != nil {
		return err
	}
	result, err := requestNodeLifecycle(ctx, record, targetenrollment.StatusPath)
	if err != nil {
		return err
	}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	state := classifyNodeTrust(result.Revoked, result.Enrolled, result.ExpiresAt, time.Now())
	fmt.Fprintf(out, "Target  %s\nNode    %s\nRuntime %s\nState   %s\n", result.TargetID, result.NodeID, result.Runtime, state)
	if !result.ExpiresAt.IsZero() {
		fmt.Fprintf(out, "Expires %s\n", result.ExpiresAt.UTC().Format(time.RFC3339))
	}
	if result.Serial != "" {
		fmt.Fprintf(out, "Serial  %s\n", result.Serial)
	}
	for _, capability := range result.Capabilities {
		flag := "available"
		if !capability.Available {
			flag = "unavailable"
		}
		if capability.Detail != "" {
			fmt.Fprintf(out, "%-14s %s: %s (%s)\n", "", capability.Name, flag, capability.Detail)
		} else {
			fmt.Fprintf(out, "%-14s %s: %s\n", "", capability.Name, flag)
		}
	}
	return nil
}

func requestNodeLifecycle(ctx context.Context, record nodeRecord, path string) (nodeStatusResult, error) {
	session, err := operatorauth.LoadSession(record.CoreTarget, record.Environment)
	if err != nil || !session.ValidAt(time.Now()) {
		return nodeStatusResult{}, machine.NewError(machine.ErrorAuthenticationFailed, "node lifecycle requires a valid Core operator session", "Run 'baha login' for the Core Target/environment and retry.", false)
	}
	client, err := nodeHTTPSClient(record.CAFile)
	if err != nil {
		return nodeStatusResult{}, err
	}
	payload, _ := json.Marshal(map[string]string{"target_id": record.TargetID, "node_id": record.NodeID, "environment": record.Environment})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(record.CoreURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return nodeStatusResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+session.IDToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nodeStatusResult{}, machine.Wrap(machine.ErrorProviderUnavailable, err, "Verify Core HTTPS reachability and trust.", true)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if err != nil || len(data) > 16384 {
		return nodeStatusResult{}, errors.New("invalid Core node lifecycle response")
	}
	if resp.StatusCode != http.StatusOK {
		return nodeStatusResult{}, machine.NewError(machine.ErrorConflict, fmt.Sprintf("Core rejected node lifecycle request (HTTP %d)", resp.StatusCode), "Verify node ownership, operator permissions and current enrollment state.", false)
	}
	if path == targetenrollment.DisconnectPath {
		return nodeStatusResult{ContractVersion: machine.ContractVersion, TargetID: record.TargetID, NodeID: record.NodeID, Runtime: record.Runtime, Revoked: true}, nil
	}
	var result nodeStatusResult
	if json.Unmarshal(data, &result) != nil {
		return nodeStatusResult{}, errors.New("invalid Core node status response")
	}
	if result.ContractVersion != machine.ContractVersion || result.TargetID != record.TargetID || result.NodeID != record.NodeID || result.Runtime != record.Runtime {
		return nodeStatusResult{}, errors.New("Core node status response did not confirm the requested target/node identity")
	}
	// A positively enrolled status without a certificate expiry is not a usable
	// lifecycle authority; fail closed instead of guessing ENROLLED.
	if result.Enrolled && result.ExpiresAt.IsZero() {
		return nodeStatusResult{}, errors.New("Core node status response reported enrollment without a certificate expiry")
	}
	// Surface live trust details the Core authority reports: certificate serial
	// and per-capability availability. Unsupported capability and unavailable
	// authority are never synthesised locally; the Core's own capability verdict
	// is the only source of truth.
	if capsRaw, ok := decodeCapabilitiesFromCoreResponse(data); ok {
		result.Capabilities = capsRaw
	}
	return result, nil
}
