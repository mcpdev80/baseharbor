package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/machinehttp"
)

// discoverMachineOperations negotiates the canonical HTTPS semantic interface.
// The Core origin and bearer were verified together at dial time. No endpoint
// URL from discovery is ever trusted as an independent remote authority.
func (c *verifiedRemoteCore) discoverMachineOperations(ctx context.Context) (machine.Discovery, error) {
	if c == nil || c.client == nil || c.origin == "" || c.token == "" || c.target == "" || c.tenant == "" {
		return machine.Discovery{}, errors.New("remote Core connection is not verified")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.origin+"/api/v1/machine/discovery", nil)
	if err != nil {
		return machine.Discovery{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
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

// requestMachineExecution dispatches only a discovered operation to the
// startup-pinned Core. Its idempotency key belongs to the caller, so a
// transport timeout never silently generates a new mutation on retry.
// A 202 is an admission acknowledgement, not an operation success.
func (c *verifiedRemoteCore) requestMachineExecution(
	ctx context.Context, operationID string, selectors machine.OperationContext,
	input json.RawMessage, idempotencyKey string,
) (machine.Execution, error) {
	if c == nil || c.client == nil || c.origin == "" || c.token == "" || strings.TrimSpace(operationID) == "" || strings.TrimSpace(idempotencyKey) == "" ||
		selectors.Target != c.target || selectors.Environment == "" {
		return machine.Execution{}, errors.New("remote machine operation requires pinned Target, environment and idempotency key")
	}
	discovery, err := c.discoverMachineOperations(ctx)
	if err != nil {
		return machine.Execution{}, err
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
		OperationID: operationID, Context: selectors, Input: input,
	})
	if err != nil {
		return machine.Execution{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/api/v1/machine/executions", bytes.NewReader(payload))
	if err != nil {
		return machine.Execution{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return machine.Execution{}, fmt.Errorf("remote machine execution result unknown; query the pinned Core before retrying with the same idempotency key: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return machine.Execution{}, fmt.Errorf("remote Core rejected machine operation (HTTP %d)", resp.StatusCode)
	}
	var execution machine.Execution
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := decoder.Decode(&execution); err != nil || decoder.Decode(new(any)) != io.EOF {
		return machine.Execution{}, errors.New("remote Core machine execution acknowledgement is invalid")
	}
	if execution.ContractVersion != machine.ExecutionContractVersion ||
		execution.OperationID != operationID ||
		execution.Context.Target != selectors.Target ||
		execution.Context.Environment != selectors.Environment ||
		execution.Context.Application != selectors.Application ||
		execution.ExecutionID == "" ||
		resp.Header.Get("Location") != "/api/v1/machine/executions/"+execution.ExecutionID {
		return machine.Execution{}, errors.New("remote Core machine execution acknowledgement does not match the requested authority")
	}
	return execution, nil
}
