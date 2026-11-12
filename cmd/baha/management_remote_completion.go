package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Presentation consumes the verified lifecycle receipt; it never infers success
// from HTTP admission or the presence of running containers.
func renderRemoteApplyCompletion(ctx context.Context, out io.Writer, raw json.RawMessage) error {
	var receipt struct {
		ContractVersion string                  `json:"contract_version"`
		Verified        bool                    `json:"verified"`
		Operation       string                  `json:"operation"`
		Application     string                  `json:"application"`
		Target          string                  `json:"target"`
		Environment     string                  `json:"environment"`
		Status          applicationStatusResult `json:"status"`
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.ContractVersion != machine.ContractVersion || receipt.Operation != "apply" || !receipt.Verified || !receipt.Status.Ready || receipt.Status.HasFailures() || receipt.Status.HasUnverified() || receipt.Application != receipt.Status.Application || receipt.Target != receipt.Status.Target || receipt.Environment != receipt.Status.Environment {
		return errors.New("Core apply result does not contain verified application readiness")
	}
	fmt.Fprintf(out, "\nREADY · %s\nTarget: %s · Environment: %s\nApplication and requested services verified.\n", receipt.Application, receipt.Target, receipt.Environment)
	seen := map[string]bool{}
	printLink := func(label, value string) {
		address, err := url.Parse(value)
		if err != nil || (address.Scheme != "https" && address.Scheme != "http") || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" || seen[address.String()] {
			return
		}
		seen[address.String()] = true
		fmt.Fprintf(out, "%s: %s\n", label, address.String())
	}
	for _, address := range receipt.Status.ApplicationURLs {
		printLink("Open (IP)", address)
	}
	for _, check := range receipt.Status.Checks {
		if check.OK && (strings.HasPrefix(check.Name, "managed-exposure/") || strings.HasPrefix(check.Name, "exposure/")) {
			printLink("Open (URL)", check.Detail)
		}
	}
	if cli.OutputOptionsFromContext(ctx).Verbose {
		renderApplicationStatus(ctx, out, out, receipt.Status.StatusResult)
	}
	fmt.Fprintln(out, "\nNext: baha status · baha doctor")
	return nil
}
