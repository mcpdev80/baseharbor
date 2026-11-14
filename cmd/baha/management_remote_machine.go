package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
)

// discoverMachineOperations negotiates the canonical HTTPS semantic interface.
// The Core origin and bearer were verified together at dial time. No endpoint
// URL from discovery is ever trusted as an independent remote authority.
func (c *verifiedRemoteCore) discoverMachineOperations(ctx context.Context) (machine.Discovery, error) {
	if c == nil || c.client == nil || c.origin == "" || c.target == "" || c.tenant == "" {
		return machine.Discovery{}, errors.New("remote Core connection is not verified")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+"/api/v1/machine/discovery", nil)
	if err != nil {
		return machine.Discovery{}, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return machine.Discovery{}, fmt.Errorf("query pinned Core machine discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return machine.Discovery{}, fmt.Errorf("remote Core machine discovery rejected (HTTP %d)", resp.StatusCode)
	}
	var discovery machine.Discovery
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&discovery); err != nil {
		return machine.Discovery{}, errors.New("remote Core machine discovery is invalid")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return machine.Discovery{}, errors.New("remote Core machine discovery has trailing data")
	}
	if discovery.ContractVersion != machine.ContractVersion || discovery.ExecutionVersion != machine.ExecutionContractVersion {
		return machine.Discovery{}, errors.New("remote Core machine contract version is unsupported")
	}
	bindings := machine.MachineHTTPBindings()
	for _, name := range []string{"discovery", "execute", "execution"} {
		got, exists := discovery.HTTP[name]
		want := bindings[name]
		if !exists || got.Href != want.Href || got.Method != want.Method {
			return machine.Discovery{}, errors.New("remote Core machine endpoint differs from the pinned canonical contract")
		}
	}
	return discovery, nil
}

func (c *verifiedRemoteCore) serverTargetName() string {
	if c == nil || strings.TrimSpace(c.coreTarget) == "" {
		if c == nil {
			return ""
		}
		return c.target
	}
	return c.coreTarget
}

// requestMachineExecution dispatches only a discovered operation to the
// startup-pinned Core. Its idempotency key belongs to the caller, so a
// transport timeout never silently generates a new mutation on retry.
// A 202 is an admission acknowledgement, not an operation success.
func (c *verifiedRemoteCore) requestMachineExecution(
	ctx context.Context, operationID string, selectors machine.OperationContext,
	input json.RawMessage, idempotencyKey string,
) (machine.Execution, error) {
	return c.requestMachineExecutionMode(ctx, operationID, selectors, input, idempotencyKey, false)
}

func (c *verifiedRemoteCore) requestMachineExecutionTerminal(
	ctx context.Context, operationID string, selectors machine.OperationContext,
	input json.RawMessage, idempotencyKey string,
) (machine.Execution, error) {
	if operationID != "installation.destroy" {
		return machine.Execution{}, errors.New("terminal execution mode is reserved for installation.destroy")
	}
	return c.requestMachineExecutionMode(ctx, operationID, selectors, input, idempotencyKey, true)
}

func (c *verifiedRemoteCore) requestMachineExecutionMode(
	ctx context.Context, operationID string, selectors machine.OperationContext,
	input json.RawMessage, idempotencyKey string, waitForTerminal bool,
) (machine.Execution, error) {
	if c == nil || c.client == nil || c.origin == "" || strings.TrimSpace(operationID) == "" || strings.TrimSpace(idempotencyKey) == "" ||
		selectors.Target != c.target || c.serverTargetName() == "" || selectors.Environment == "" {
		return machine.Execution{}, errors.New("remote machine operation requires pinned Target, environment and idempotency key")
	}
	selectors.Target = c.serverTargetName()
	input, err := c.inputWithCoreTarget(input)
	if err != nil {
		return machine.Execution{}, err
	}
	discovery, err := c.discoverMachineOperations(ctx)
	if err != nil {
		return machine.Execution{}, err
	}
	if waitForTerminal {
		terminalCapable := false
		for _, capability := range discovery.Capabilities {
			terminalCapable = terminalCapable || capability == machinehttp.CapabilityTerminalInstallationDestroy
		}
		if !terminalCapable {
			return machine.Execution{}, errors.New("remote Core does not support authenticated terminal receipts for installation.destroy; update the Core before retrying")
		}
	}
	supported := false
	for _, operation := range discovery.Operations {
		if operation.ID == operationID {
			supported = true
			break
		}
	}
	if !supported {
		return machine.Execution{}, errors.New("remote Core does not advertise requested machine operation")
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	payload, err := json.Marshal(machinehttp.ExecuteRequest{
		OperationID: operationID, Context: selectors, Input: input, WaitForTerminal: waitForTerminal,
	})
	if err != nil {
		return machine.Execution{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/api/v1/machine/executions", bytes.NewReader(payload))
	if err != nil {
		return machine.Execution{}, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	// Machine HTTP may synchronously commit a long-running lifecycle before
	// finishing its HTTP 202 response. The identity/discovery client uses a
	// short timeout; only the authorized execute request uses the Core's
	// 30-minute lifecycle budget plus a small transport margin.
	executionClient := *c.client
	executionClient.Timeout = 31 * time.Minute
	resp, err := executionClient.Do(req)
	if err != nil {
		return machine.Execution{}, fmt.Errorf("remote machine execution result unknown; query the pinned Core before retrying with the same idempotency key: %w", err)
	}
	defer resp.Body.Close()
	wantStatus := http.StatusAccepted
	if waitForTerminal {
		wantStatus = http.StatusOK
	}
	if resp.StatusCode != wantStatus {
		return machine.Execution{}, fmt.Errorf("remote Core rejected machine operation (HTTP %d)", resp.StatusCode)
	}
	var execution machine.Execution
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&execution); err != nil || decoder.Decode(new(any)) != io.EOF {
		return machine.Execution{}, errors.New("remote Core machine execution acknowledgement is invalid")
	}
	if waitForTerminal {
		if err := execution.Validate(); err != nil {
			return machine.Execution{}, fmt.Errorf("remote Core terminal execution receipt is invalid: %w", err)
		}
	}
	if execution.ContractVersion != machine.ExecutionContractVersion ||
		execution.OperationID != operationID ||
		execution.Context.Target != c.serverTargetName() ||
		execution.Context.Environment != selectors.Environment ||
		execution.Context.Application != selectors.Application ||
		execution.ExecutionID == "" ||
		resp.Header.Get("Location") != "/api/v1/machine/executions/"+execution.ExecutionID {
		return machine.Execution{}, errors.New("remote Core machine execution acknowledgement does not match the requested authority")
	}
	if waitForTerminal && execution.State != machine.ExecutionSucceeded && execution.State != machine.ExecutionFailed {
		return machine.Execution{}, errors.New("remote Core did not return a terminal installation destroy result")
	}
	execution.Context.Target = c.target
	return execution, nil
}

func (c *verifiedRemoteCore) inputWithCoreTarget(input json.RawMessage) (json.RawMessage, error) {
	if len(input) == 0 {
		return input, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil || fields == nil {
		return nil, errors.New("remote machine operation input is invalid")
	}
	if raw, ok := fields["target"]; ok {
		var supplied string
		if err := json.Unmarshal(raw, &supplied); err != nil || supplied != c.target && supplied != c.coreTarget {
			return nil, errors.New("remote machine operation input Target differs from the selected workstation Target")
		}
		value, _ := json.Marshal(c.serverTargetName())
		fields["target"] = value
		return json.Marshal(fields)
	}
	return input, nil
}

var remoteExecutionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

var errRemoteExecutionPending = errors.New("remote Core machine operation is not terminal; no success may be reported")

const maxRemoteMachineExecutionResponseBytes = 8 << 20

// readMachineExecution observes a previously admitted operation over the same
// pinned Core origin and bearer. Never follow a server-provided URL.
func (c *verifiedRemoteCore) readMachineExecution(ctx context.Context, admitted machine.Execution) (machine.Execution, error) {
	if c == nil || c.client == nil || c.origin == "" || c.target == "" || c.serverTargetName() == "" || c.tenant == "" ||
		!remoteExecutionIDPattern.MatchString(admitted.ExecutionID) || admitted.Context.Target != c.target ||
		admitted.Context.Environment == "" || admitted.OperationID == "" {
		return machine.Execution{}, errors.New("remote execution is not bound to the verified Core and Target")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.origin+"/api/v1/machine/executions/"+admitted.ExecutionID, nil)
	if err != nil {
		return machine.Execution{}, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return machine.Execution{}, fmt.Errorf("read pinned Core machine execution: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return machine.Execution{}, fmt.Errorf("remote Core machine execution read rejected (HTTP %d)", resp.StatusCode)
	}
	var execution machine.Execution
	response, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteMachineExecutionResponseBytes+1))
	if err != nil {
		return machine.Execution{}, fmt.Errorf("read remote Core machine execution response: %w", err)
	}
	if len(response) > maxRemoteMachineExecutionResponseBytes {
		return machine.Execution{}, fmt.Errorf("remote Core machine execution response exceeds %d bytes", maxRemoteMachineExecutionResponseBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(response))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&execution); err != nil {
		return machine.Execution{}, fmt.Errorf("remote Core machine execution response is invalid: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return machine.Execution{}, errors.New("remote Core machine execution response contains trailing data")
	}
	if execution.Context.Target == c.serverTargetName() {
		execution.Context.Target = c.target
	}
	if err := execution.Validate(); err != nil ||
		execution.ExecutionID != admitted.ExecutionID ||
		execution.OperationID != admitted.OperationID ||
		execution.Context != admitted.Context {
		return machine.Execution{}, errors.New("remote Core machine execution response changed the accepted identity or scope")
	}
	return execution, nil
}

// confirmRemoteExecutionSuccess never treats HTTP admission or a pending
// execution as a completed deployment. A caller may retry the read only;
// mutation retries must keep the original idempotency key.
func (c *verifiedRemoteCore) confirmRemoteExecutionSuccess(ctx context.Context, admitted machine.Execution) (json.RawMessage, error) {
	execution, err := c.readMachineExecution(ctx, admitted)
	if err != nil {
		return nil, err
	}
	if execution.Progress != nil {
		reportRemoteProgress(ctx, execution.Progress.Message)
	}
	switch execution.State {
	case machine.ExecutionSucceeded:
		return rewriteRemoteResultTarget(execution.Result, c.serverTargetName(), c.target)
	case machine.ExecutionFailed:
		return nil, execution.Error
	case machine.ExecutionCancelled:
		return nil, errors.New("remote Core machine operation was cancelled")
	default:
		return nil, errRemoteExecutionPending
	}
}

func rewriteRemoteResultTarget(result json.RawMessage, coreTarget, workstationTarget string) (json.RawMessage, error) {
	if len(result) == 0 || coreTarget == workstationTarget {
		return result, nil
	}
	var value any
	if err := json.Unmarshal(result, &value); err != nil {
		return nil, errors.New("remote Core result is invalid JSON")
	}
	var rewrite func(any)
	rewrite = func(current any) {
		switch item := current.(type) {
		case map[string]any:
			for key, child := range item {
				if key == "target" {
					if name, ok := child.(string); ok && name == coreTarget {
						item[key] = workstationTarget
						continue
					}
				}
				rewrite(child)
			}
		case []any:
			for _, child := range item {
				rewrite(child)
			}
		}
	}
	rewrite(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("encode remote Core result for workstation Target")
	}
	return encoded, nil
}
