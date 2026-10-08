package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

const nodeConnectorProvider = "baseharbor-node-connector"
const nodeBundleVersion = "baseharbor.node-enrollment-bundle/v1"

var nodeInput io.Reader = os.Stdin

type nodeEnrollmentBundle struct {
	ContractVersion string                     `json:"contract_version"`
	TenantID        string                     `json:"tenant_id"`
	TargetID        string                     `json:"target_id"`
	NodeID          string                     `json:"node_id"`
	Runtime         string                     `json:"runtime"`
	Environment     string                     `json:"environment"`
	CoreURL         string                     `json:"core_url"`
	CoreAddress     string                     `json:"core_address"`
	ServerName      string                     `json:"server_name"`
	BootstrapCA     string                     `json:"bootstrap_ca_pem"`
	Authorization   targetenrollment.Bootstrap `json:"authorization"`
}

type nodeRecord struct {
	ContractVersion string `json:"contract_version"`
	TenantID        string `json:"tenant_id"`
	TargetID        string `json:"target_id"`
	NodeID          string `json:"node_id"`
	Runtime         string `json:"runtime"`
	Environment     string `json:"environment"`
	CoreTarget      string `json:"core_target"`
	CoreURL         string `json:"core_url"`
	CoreAddress     string `json:"core_address"`
	ServerName      string `json:"server_name"`
	CAFile          string `json:"ca_file"`
}

type nodeListItem struct {
	TargetID string `json:"target_id"`
	NodeID   string `json:"node_id"`
	TenantID string `json:"tenant_id,omitempty"`
	Runtime  string `json:"runtime"`
}

type nodeAddResult struct {
	ContractVersion string    `json:"contract_version"`
	TargetID        string    `json:"target_id"`
	NodeID          string    `json:"node_id"`
	Runtime         string    `json:"runtime"`
	Authorization   string    `json:"authorization_file"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type nodeStatusResult struct {
	ContractVersion string    `json:"contract_version"`
	TargetID        string    `json:"target_id"`
	NodeID          string    `json:"node_id"`
	Runtime         string    `json:"runtime"`
	Enrolled        bool      `json:"enrolled"`
	Revoked         bool      `json:"revoked"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
}

type nodeConnectResult struct {
	ContractVersion string `json:"contract_version"`
	TargetID        string `json:"target_id"`
	NodeID          string `json:"node_id"`
	Service         string `json:"service"`
	State           string `json:"state"`
}

func nodeCommand() *cli.Command {
	return &cli.Command{
		Name:    "node",
		Summary: "Enroll and manage remote Docker or Podman nodes",
		Usage:   "baha node [add|connect|list|status|disconnect]",
		Children: []*cli.Command{
			{Name: "add", Summary: "Create a remote Target and one-use enrollment bundle", Usage: "baha node add [NODE] [--target NAME] --runtime docker|podman --tenant-id UUID --core-url https://HOST:PORT --core-address HOST:PORT --ca-file FILE [--environment ENV] [--output FILE] [--json]", Run: nodeAdd},
			{Name: "connect", Summary: "Enroll this host and start the rootless connector service", Usage: "baha node connect [ENROLLMENT-FILE] [--connector-bin FILE] [--json]", Run: nodeConnect},
			{Name: "list", Summary: "List configured remote connector nodes", Usage: "baha node list [--json]", Run: nodeList},
			{Name: "status", Summary: "Inspect Core enrollment state for a remote node", Usage: "baha node status TARGET [--json]", Run: nodeStatus},
			{Name: "disconnect", Summary: "Revoke a node identity and remove its empty Target registration", Usage: "baha node disconnect TARGET --yes [--json]", Run: nodeDisconnect},
		},
	}
}

func nodeAdd(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "node add")
	if err != nil {
		return err
	}
	opts, err := parseNodeAddArgs(filtered)
	if err != nil {
		return err
	}
	if opts.NodeID == "" {
		if noInput(ctx) || !readerIsTerminal(nodeInput) {
			return usageError("baha node add requires NODE in non-interactive mode", "Provide NODE plus --runtime, --tenant-id, --core-url, --core-address and --ca-file.")
		}
		opts, err = promptNodeAdd(opts, out)
		if err != nil {
			return err
		}
	}
	if opts.TargetID == "" {
		opts.TargetID = opts.NodeID
	}
	if opts.Environment == "" {
		opts.Environment = "dev"
	}
	if err := validateNodeAddOptions(opts); err != nil {
		return err
	}
	coreTarget, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	if coreTarget.AccessProvider != "local" {
		return usageError("node add must run against a local Core authority Target", "Select the local Core with 'baha target activate NAME' and retry.")
	}
	caPEM, err := os.ReadFile(opts.CAFile)
	if err != nil {
		return fmt.Errorf("read Core CA: %w", err)
	}
	if !validCAPEM(caPEM) {
		return errors.New("Core CA file contains no valid CA certificate")
	}

	created, err := createTargetDefinition(ctx, machineTargetCreateInput{
		Name: opts.TargetID, TenantID: opts.TenantID, RuntimeProvider: opts.Runtime,
		AccessProvider: nodeConnectorProvider, Access: "node-" + opts.NodeID, Reference: opts.NodeID,
	})
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback && created.Created {
			_ = rollbackNodeTarget(opts.TargetID)
		}
	}()

	grant, err := requestNodeAuthorization(ctx, coreTarget.Name, opts.Environment, opts.CoreURL, opts.CAFile, opts.TargetID, opts.NodeID)
	if err != nil {
		return err
	}
	serverName, err := nodeServerName(opts.CoreAddress)
	if err != nil {
		return err
	}
	bundle := nodeEnrollmentBundle{
		ContractVersion: nodeBundleVersion, TenantID: opts.TenantID, TargetID: opts.TargetID, NodeID: opts.NodeID,
		Runtime: opts.Runtime, Environment: opts.Environment, CoreTarget: coreTarget.Name, CoreURL: strings.TrimRight(opts.CoreURL, "/"),
		CoreAddress: opts.CoreAddress, ServerName: serverName, BootstrapCA: string(caPEM), Authorization: grant,
	}
	if opts.Output == "" {
		opts.Output, err = defaultNodeBundlePath(opts.TargetID)
		if err != nil {
			return err
		}
	}
	if err := writePrivateJSON(opts.Output, bundle); err != nil {
		return err
	}
	record := nodeRecord{
		ContractVersion: machine.ContractVersion, TenantID: opts.TenantID, TargetID: opts.TargetID, NodeID: opts.NodeID,
		Runtime: opts.Runtime, Environment: opts.Environment, CoreURL: strings.TrimRight(opts.CoreURL, "/"),
		CoreAddress: opts.CoreAddress, ServerName: serverName, CAFile: opts.CAFile,
	}
	if err := saveNodeRecord(record); err != nil {
		_ = os.Remove(opts.Output)
		return err
	}
	rollback = false
	result := nodeAddResult{ContractVersion: machine.ContractVersion, TargetID: opts.TargetID, NodeID: opts.NodeID, Runtime: opts.Runtime, Authorization: opts.Output, ExpiresAt: grant.ExpiresAt}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "Node %s prepared as Target %s.\n", opts.NodeID, opts.TargetID)
	fmt.Fprintf(out, "Enrollment file: %s\n", opts.Output)
	fmt.Fprintln(out, "Copy this one owner-only file to the remote host, then run: baha node connect <file>")
	return nil
}

type nodeAddOptions struct {
	NodeID, TargetID, Runtime, TenantID, CoreURL, CoreAddress, CAFile, Environment, Output string
}

func parseNodeAddArgs(args []string) (nodeAddOptions, error) {
	var o nodeAddOptions
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		o.NodeID = strings.TrimSpace(args[0])
		args = args[1:]
	}
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return o, usageError(args[i]+" requires a value", "Run 'baha node add --help' for usage.")
		}
		key, value := args[i], strings.TrimSpace(args[i+1])
		i++
		switch key {
		case "--target":
			o.TargetID = value
		case "--runtime":
			o.Runtime = value
		case "--tenant-id":
			o.TenantID = value
		case "--core-url":
			o.CoreURL = value
		case "--core-address":
			o.CoreAddress = value
		case "--ca-file":
			o.CAFile = value
		case "--environment":
			o.Environment = strings.ToLower(value)
		case "--output":
			o.Output = value
		default:
			return o, unknownOptionUsage("baha node add", key, "--target", "--runtime", "--tenant-id", "--core-url", "--core-address", "--ca-file", "--environment", "--output", "--json")
		}
	}
	return o, nil
}

func promptNodeAdd(o nodeAddOptions, out io.Writer) (nodeAddOptions, error) {
	reader := bufioNewReader(nodeInput)
	var err error
	if o.NodeID, err = promptLine(reader, out, "Node name", o.NodeID); err != nil {
		return o, err
	}
	if o.TargetID, err = promptLine(reader, out, "Target name", o.NodeID); err != nil {
		return o, err
	}
	if o.Runtime, err = promptLine(reader, out, "Runtime (docker/podman)", "podman"); err != nil {
		return o, err
	}
	if o.TenantID, err = promptLine(reader, out, "Tenant UUID", ""); err != nil {
		return o, err
	}
	if o.CoreURL, err = promptLine(reader, out, "Core HTTPS API URL", "https://127.0.0.1:8443"); err != nil {
		return o, err
	}
	if o.CoreAddress, err = promptLine(reader, out, "Connector session address", "127.0.0.1:9443"); err != nil {
		return o, err
	}
	if o.CAFile, err = promptLine(reader, out, "Core public CA file", ""); err != nil {
		return o, err
	}
	if o.Environment, err = promptLine(reader, out, "Environment", "dev"); err != nil {
		return o, err
	}
	return o, nil
}

// Kept as a tiny seam so tests can replace buffered input without changing prompt semantics.
var bufioNewReader = func(r io.Reader) *bufio.Reader { return bufio.NewReader(r) }

func validateNodeAddOptions(o nodeAddOptions) error {
	if err := deployment.ValidateTargetName(o.TargetID); err != nil {
		return err
	}
	if strings.TrimSpace(o.NodeID) == "" || strings.ContainsAny(o.NodeID, "/\\ \t\r\n") {
		return errors.New("node name must be a bounded identity segment without whitespace or path separators")
	}
	if o.Runtime != "docker" && o.Runtime != "podman" {
		return usageError("--runtime must be docker or podman", "Choose the runtime installed on the remote node.")
	}
	if !canonicalTenantID(o.TenantID) {
		return usageError("--tenant-id must be a canonical lowercase UUID", "Use the tenant ID owned by the Core operator membership.")
	}
	if err := validateCoreURL(o.CoreURL); err != nil {
		return err
	}
	if _, _, err := net.SplitHostPort(o.CoreAddress); err != nil {
		return usageError("--core-address must use host:port form", "Provide the outbound Connector listener, for example core.example:9443.")
	}
	if strings.TrimSpace(o.CAFile) == "" {
		return usageError("--ca-file is required", "Provide the public CA bundle that authenticates the Core HTTPS endpoint.")
	}
	return nil
}

func canonicalTenantID(value string) bool {
	if len(value) != 36 || value != strings.ToLower(value) {
		return false
	}
	for i, r := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func validateCoreURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return usageError("--core-url must be an absolute HTTPS URL without credentials, query or fragment", "Example: https://core.example:8443")
	}
	if u.Path != "" && u.Path != "/" {
		return usageError("--core-url must not include an API path", "Provide only the Core HTTPS origin.")
	}
	return nil
}

func requestNodeAuthorization(ctx context.Context, coreTarget, environment, coreURL, caFile, targetID, nodeID string) (targetenrollment.Bootstrap, error) {
	session, err := operatorauth.LoadSession(coreTarget, environment)
	if err != nil || !session.ValidAt(time.Now()) {
		return targetenrollment.Bootstrap{}, machine.NewError(machine.ErrorAuthenticationFailed,
			"node enrollment requires a valid Core operator session",
			"Run 'baha login' for the selected Core Target/environment and retry.", false)
	}
	client, err := nodeHTTPSClient(caFile)
	if err != nil {
		return targetenrollment.Bootstrap{}, err
	}
	payload, _ := json.Marshal(map[string]any{
		"target_id": targetID, "node_id": nodeID, "environment": environment,
		"lifetime_seconds": 600, "certificate_ttl_seconds": 86400,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(coreURL, "/")+targetenrollment.AuthorizationPath, bytes.NewReader(payload))
	if err != nil {
		return targetenrollment.Bootstrap{}, err
	}
	req.Header.Set("Authorization", "Bearer "+session.IDToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return targetenrollment.Bootstrap{}, machine.Wrap(machine.ErrorProviderUnavailable, err, "Verify Core HTTPS reachability and trust.", true)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16385))
	if err != nil || len(data) > 16384 {
		return targetenrollment.Bootstrap{}, errors.New("invalid Core enrollment authorization response")
	}
	if resp.StatusCode != http.StatusCreated {
		return targetenrollment.Bootstrap{}, machine.NewError(machine.ErrorAuthenticationFailed,
			fmt.Sprintf("Core rejected node enrollment authorization (HTTP %d)", resp.StatusCode),
			"Verify operator permissions, tenant ownership and Target scope.", false)
	}
	var grant targetenrollment.Bootstrap
	if json.Unmarshal(data, &grant) != nil || grant.Token == "" || grant.Nonce == "" || !grant.ExpiresAt.After(time.Now()) {
		return targetenrollment.Bootstrap{}, errors.New("Core returned an invalid node enrollment authorization")
	}
	return grant, nil
}

func nodeHTTPSClient(caFile string) (*http.Client, error) {
	pemData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemData) {
		return nil, errors.New("Core CA file contains no certificates")
	}
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

func validCAPEM(data []byte) bool {
	pool := x509.NewCertPool()
	return pool.AppendCertsFromPEM(data)
}

func nodeServerName(address string) (string, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	return strings.Trim(host, "[]"), nil
}

func defaultNodeBundlePath(target string) (string, error) {
	root, err := deployment.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "node-enrollment", target+".json"), nil
}

func nodeRecordPath(target string) (string, error) {
	root, err := deployment.DataRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "nodes", target+".json"), nil
}

func saveNodeRecord(record nodeRecord) error {
	path, err := nodeRecordPath(record.TargetID)
	if err != nil {
		return err
	}
	return writePrivateJSON(path, record)
}

func loadNodeRecord(target string) (nodeRecord, error) {
	path, err := nodeRecordPath(target)
	if err != nil {
		return nodeRecord{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nodeRecord{}, err
	}
	var record nodeRecord
	if json.Unmarshal(data, &record) != nil || record.ContractVersion != machine.ContractVersion || record.TargetID != target {
		return nodeRecord{}, errors.New("invalid local node registration metadata")
	}
	return record, nil
}

func writePrivateJSON(path string, value any) error {
	path = filepath.Clean(path)
	if path == "." || path == "" {
		return errors.New("destination path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".baseharbor-node-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func rollbackNodeTarget(target string) error {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	definition, ok := cfg.Targets[target]
	if !ok {
		return nil
	}
	access := definition.Access.Reference
	delete(cfg.Targets, target)
	if cfg.DefaultTarget == target {
		cfg.DefaultTarget = ""
	}
	inUse := false
	for _, other := range cfg.Targets {
		if other.Access.Reference == access {
			inUse = true
			break
		}
	}
	if !inUse {
		delete(cfg.Access, access)
	}
	return cfg.Save()
}

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
		items = append(items, nodeListItem{TargetID: name, NodeID: access.Reference, TenantID: target.TenantID, Runtime: target.Runtime.Provider})
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
		fmt.Fprintf(out, "%-24s %-24s %-8s %s\n", item.TargetID, item.NodeID, item.Runtime, item.TenantID)
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
	state := "PENDING"
	if result.Revoked {
		state = "REVOKED"
	} else if result.Enrolled && result.ExpiresAt.After(time.Now()) {
		state = "ENROLLED"
	}
	fmt.Fprintf(out, "Target  %s\nNode    %s\nRuntime %s\nState   %s\n", result.TargetID, result.NodeID, result.Runtime, state)
	if !result.ExpiresAt.IsZero() {
		fmt.Fprintf(out, "Expires %s\n", result.ExpiresAt.UTC().Format(time.RFC3339))
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
	return result, nil
}

func effectiveCoreSessionTarget() string {
	cfg, err := deployment.LoadConfig()
	if err == nil {
		if active, activeErr := readPersistedTarget(); activeErr == nil && active != "" {
			if target, ok := cfg.Targets[active]; ok {
				if access, ok := cfg.Access[target.Access.Reference]; ok && access.Provider == "local" {
					return active
				}
			}
		}
		if cfg.DefaultTarget != "" {
			if target, ok := cfg.Targets[cfg.DefaultTarget]; ok {
				if access, ok := cfg.Access[target.Access.Reference]; ok && access.Provider == "local" {
					return cfg.DefaultTarget
				}
			}
		}
	}
	return "local"
}

func nodeDisconnect(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "node disconnect")
	if err != nil {
		return err
	}
	yes := false
	var positional []string
	for _, arg := range filtered {
		if arg == "--yes" {
			yes = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return unknownOptionUsage("baha node disconnect", arg, "--yes", "--json")
		}
		positional = append(positional, arg)
	}
	if len(positional) != 1 {
		return usageError("baha node disconnect requires TARGET", "Run 'baha node list' to choose the remote Target.")
	}
	if !yes {
		return machine.NewError(machine.ErrorApprovalRequired, "node disconnect requires explicit approval", "Re-run with --yes after reviewing the Target.", false)
	}
	record, err := loadNodeRecord(positional[0])
	if err != nil {
		return err
	}
	if _, err := requestNodeLifecycle(ctx, record, targetenrollment.DisconnectPath); err != nil {
		return err
	}
	if _, err := deleteTargetDefinition(ctx, record.TargetID); err != nil {
		return fmt.Errorf("node identity revoked but Target cleanup failed: %w", err)
	}
	path, _ := nodeRecordPath(record.TargetID)
	_ = os.Remove(path)
	result := map[string]any{"contract_version": machine.ContractVersion, "target_id": record.TargetID, "node_id": record.NodeID, "revoked": true, "deleted": true}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "Node %s disconnected and Target %s removed.\n", record.NodeID, record.TargetID)
	return nil
}

func nodeConnect(ctx context.Context, args []string, out, errOut io.Writer) error {
	filtered, format, err := parseReadOutputArgs(args, "node connect")
	if err != nil {
		return err
	}
	var bundlePath, connectorBin string
	for i := 0; i < len(filtered); i++ {
		if filtered[i] == "--connector-bin" {
			if i+1 >= len(filtered) {
				return usageError("--connector-bin requires a value", "Provide the installed baseharbor-node-connector executable.")
			}
			connectorBin = filtered[i+1]
			i++
			continue
		}
		if strings.HasPrefix(filtered[i], "-") {
			return unknownOptionUsage("baha node connect", filtered[i], "--connector-bin", "--json")
		}
		if bundlePath != "" {
			return usageError("baha node connect accepts one enrollment file", "Run 'baha node connect <file>'.")
		}
		bundlePath = filtered[i]
	}
	if bundlePath == "" {
		if noInput(ctx) || !readerIsTerminal(nodeInput) {
			return usageError("baha node connect requires an enrollment file in non-interactive mode", "Copy the file produced by 'baha node add' and pass its path.")
		}
		reader := bufioNewReader(nodeInput)
		bundlePath, err = promptLine(reader, out, "Enrollment file", "")
		if err != nil {
			return err
		}
	}
	bundle, err := readNodeBundle(bundlePath)
	if err != nil {
		return err
	}
	if connectorBin == "" {
		connectorBin, err = exec.LookPath("baseharbor-node-connector")
		if err != nil {
			return machine.NewError(machine.ErrorNotFound, "baseharbor-node-connector is not installed", "Install the connector binary from its signed release and retry.", false)
		}
	}
	stateRoot, err := connectorStateRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return err
	}
	caPath := filepath.Join(stateRoot, "bootstrap-ca.pem")
	authPath := filepath.Join(stateRoot, "bootstrap.authorization.json")
	if err := os.WriteFile(caPath, []byte(bundle.BootstrapCA), 0o600); err != nil {
		return err
	}
	if err := writePrivateJSON(authPath, bundle.Authorization); err != nil {
		return err
	}
	unitPath, err := installNodeUserUnit(connectorBin, stateRoot, caPath, authPath, bundle)
	if err != nil {
		return err
	}
	if err := runNodeCommand(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := runNodeCommand(ctx, "systemctl", "--user", "enable", "--now", filepath.Base(unitPath)); err != nil {
		return err
	}
	if err := runNodeCommand(ctx, "systemctl", "--user", "is-active", "--quiet", filepath.Base(unitPath)); err != nil {
		return machine.Wrap(machine.ErrorRuntimeUnavailable, err, "Inspect 'systemctl --user status baseharbor-node-connector.service' and connector logs.", true)
	}
	_ = os.Remove(bundlePath)
	result := nodeConnectResult{ContractVersion: machine.ContractVersion, TargetID: bundle.TargetID, NodeID: bundle.NodeID, Service: filepath.Base(unitPath), State: "connected"}
	if format == outputJSON {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "Node %s connected as Target %s.\n", bundle.NodeID, bundle.TargetID)
	fmt.Fprintln(out, "Connector service: baseharbor-node-connector.service")
	return nil
}

func readNodeBundle(path string) (nodeEnrollmentBundle, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nodeEnrollmentBundle{}, errors.New("enrollment file must be a private regular file with mode 0600")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 256<<10 {
		return nodeEnrollmentBundle{}, errors.New("cannot read bounded enrollment file")
	}
	var bundle nodeEnrollmentBundle
	if json.Unmarshal(data, &bundle) != nil || bundle.ContractVersion != nodeBundleVersion ||
		!canonicalTenantID(bundle.TenantID) || bundle.TargetID == "" || bundle.NodeID == "" ||
		(bundle.Runtime != "docker" && bundle.Runtime != "podman") || !bundle.Authorization.ExpiresAt.After(time.Now()) {
		return nodeEnrollmentBundle{}, errors.New("enrollment file is invalid or expired")
	}
	if err := validateCoreURL(bundle.CoreURL); err != nil {
		return nodeEnrollmentBundle{}, err
	}
	if !validCAPEM([]byte(bundle.BootstrapCA)) {
		return nodeEnrollmentBundle{}, errors.New("enrollment file contains no valid bootstrap CA")
	}
	return bundle, nil
}

func connectorStateRoot() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); root != "" {
		return filepath.Join(root, "baseharbor-node-connector"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "baseharbor-node-connector"), nil
}

func installNodeUserUnit(connectorBin, stateRoot, caPath, authPath string, bundle nodeEnrollmentBundle) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		return "", err
	}
	unitPath := filepath.Join(unitDir, "baseharbor-node-connector.service")
	args := []string{
		connectorBin,
		"--runtime", bundle.Runtime,
		"--core", bundle.CoreAddress,
		"--server-name", bundle.ServerName,
		"--tenant-id", bundle.TenantID,
		"--target-id", bundle.TargetID,
		"--node-id", bundle.NodeID,
		"--state-root", stateRoot,
		"--bootstrap-url", strings.TrimRight(bundle.CoreURL, "/") + targetenrollment.EnrollmentPath,
		"--bootstrap-ca", caPath,
		"--bootstrap-authorization-file", authPath,
	}
	for _, value := range args {
		if strings.ContainsAny(value, "\r\n") {
			return "", errors.New("connector service argument contains a newline")
		}
	}
	var execLine []string
	for _, value := range args {
		execLine = append(execLine, systemdQuote(value))
	}
	unit := "[Unit]\nDescription=BaseHarbor Node Connector\nAfter=network-online.target\nWants=network-online.target\n\n" +
		"[Service]\nType=simple\nExecStart=" + strings.Join(execLine, " ") + "\nRestart=on-failure\nRestartSec=5s\nNoNewPrivileges=yes\nPrivateTmp=yes\nProtectSystem=strict\nProtectHome=read-only\nReadWritePaths=" + systemdQuote(stateRoot) + "\n\n" +
		"[Install]\nWantedBy=default.target\n"
	tmp := unitPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(unit), 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, unitPath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return unitPath, nil
}

func systemdQuote(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return "\"" + value + "\""
}

var runNodeCommand = func(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 512 {
			message = message[:512]
		}
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s failed: %s", name, message)
	}
	return nil
}
