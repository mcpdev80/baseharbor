package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/mcpdev80/baseharbor/internal/machine"
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
