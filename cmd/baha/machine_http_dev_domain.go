package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

type machineHTTPDevelopmentDomainInput struct {
	Target string `json:"target,omitempty"`
	Domain string `json:"domain,omitempty"`
}
type machineHTTPDevelopmentDomainResult struct {
	ContractVersion string `json:"contract_version"`
	Target          string `json:"target"`
	Environment     string `json:"environment"`
	Domain          string `json:"domain"`
}

func executeHTTPDevelopmentDomain(_ context.Context, scope machine.OperationContext, raw json.RawMessage) (any, error) {
	var input machineHTTPDevelopmentDomainInput
	if err := decodeHTTPInput(raw, &input); err != nil {
		return nil, err
	}
	target, err := bindHTTPSelector("target", scope.Target, input.Target)
	if err != nil {
		return nil, err
	}
	if target == "" || scope.Environment != "dev" {
		return nil, machine.NewError(machine.ErrorValidationFailed, "Development domain requires the authorized development Target.", "Select the development Target.", false)
	}
	var domain string
	if input.Domain != "" {
		domain, err = devaccess.ConfigureDomain(target, input.Domain)
	} else {
		domain, err = devaccess.LoadDomain(target)
		if errors.Is(err, os.ErrNotExist) {
			domain, err = devaccess.DefaultDomain, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return machineHTTPDevelopmentDomainResult{ContractVersion: machine.ContractVersion, Target: target, Environment: "dev", Domain: domain}, nil
}
