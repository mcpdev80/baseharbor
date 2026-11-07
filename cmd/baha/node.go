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
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/operatorauth"
	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

// The node CLI asks Core for a one-use grant that lasts long enough to copy the
// enrollment file and start the connector, and for the certificate lifetime the
// connector's own renewal bound accepts (24 hours).
const (
	nodeGrantLifetime       = 600 * time.Second
	nodeCertificateLifetime = 24 * time.Hour
	nodeConnectorProvider   = "baseharbor-node-connector"
	nodeBundleVersion       = "baseharbor.node-enrollment-bundle/v1"
)

var nodeInput io.Reader = os.Stdin

type nodeEnrollmentBundle struct {
	ContractVersion string                     `json:"contract_version"`
	TenantID        string                     `json:"tenant_id"`
	TargetID        string                     `json:"target_id"`
	NodeID          string                     `json:"node_id"`
	Runtime         string                     `json:"runtime"`
	Environment     string                     `json:"environment"`
	CoreTarget      string                     `json:"core_target"`
	CoreURL         string                     `json:"core_url"`
	CoreAddress     string                     `json:"core_address"`
	ServerName      string                     `json:"server_name"`
	BootstrapCA     string                     `json:"bootstrap_ca_pem"`
	Authorization   targetenrollment.Bootstrap `json:"authorization"`
}

// nodeRecord is local registration metadata only. It deliberately stores no
// enrollment, expiry or revocation claim: the sole authority for those is the
// Core connector_nodes row, read through the lifecycle status endpoint.
type nodeRecord struct {
	ContractVersion string    `json:"contract_version"`
	TenantID        string    `json:"tenant_id"`
	TargetID        string    `json:"target_id"`
	NodeID          string    `json:"node_id"`
	Runtime         string    `json:"runtime"`
	Environment     string    `json:"environment"`
	CoreTarget      string    `json:"core_target"`
	CoreURL         string    `json:"core_url"`
	CoreAddress     string    `json:"core_address"`
	ServerName      string    `json:"server_name"`
	CAFile          string    `json:"ca_file"`
	GrantExpiresAt  time.Time `json:"grant_expires_at,omitempty"`
}

type nodeListItem struct {
	TargetID  string    `json:"target_id"`
	NodeID    string    `json:"node_id"`
	TenantID  string    `json:"tenant_id,omitempty"`
	Runtime   string    `json:"runtime"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// unconfirmed records why the lifecycle authority could not be read; it is
	// local diagnostics for the human table only, never a wire field.
	unconfirmed string
}

// classifyNodeTrust is the single production state rule shared by node list and
// node status. Positive labels require the Core lifecycle authority: an issued,
// unexpired certificate is ENROLLED, a past certificate expiry is EXPIRED, and
// revocation outranks all of them. Grant or session expiry can never produce a
// positive label, and an unconfirmed redemption stays PENDING.
func classifyNodeTrust(revoked, enrolled bool, expiresAt, now time.Time) string {
	switch {
	case revoked:
		return "REVOKED"
	case enrolled && !expiresAt.After(now):
		return "EXPIRED"
	case enrolled:
		return "ENROLLED"
	default:
		return "PENDING"
	}
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
	ContractVersion string           `json:"contract_version"`
	TargetID        string           `json:"target_id"`
	NodeID          string           `json:"node_id"`
	Runtime         string           `json:"runtime"`
	Enrolled        bool             `json:"enrolled"`
	Revoked         bool             `json:"revoked"`
	ExpiresAt       time.Time        `json:"expires_at,omitempty"`
	Serial          string           `json:"serial,omitempty"`
	Capabilities    []nodeCapability `json:"capabilities,omitempty"`
}

type nodeCapability struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
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
			{Name: "add", Summary: "Create a remote Target and one-use enrollment bundle", Usage: "baha node add [NODE] [--target NAME] --runtime docker|podman --core-url https://HOST:PORT --core-address HOST:PORT --ca-file FILE [--tenant-id UUID] [--environment ENV] [--output FILE] [--json]", Run: nodeAdd},
			{Name: "connect", Summary: "Enroll this host and start the rootless connector service", Usage: "baha node connect [ENROLLMENT-FILE] [--connector-bin FILE] [--json]", Run: nodeConnect},
			{Name: "list", Summary: "List configured remote connector nodes", Usage: "baha node list [--json]", Run: nodeList},
			{Name: "status", Summary: "Inspect Core enrollment state for a remote node", Usage: "baha node status TARGET [--json]", Run: nodeStatus},
			{Name: "disconnect", Summary: "Revoke node access after impact acknowledgement and retain recovery owners", Usage: "baha node disconnect TARGET --yes [--json]", Run: nodeDisconnect},
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
			return usageError("baha node add requires NODE in non-interactive mode", "Provide NODE plus --runtime; Core authority and trust settings must be discoverable or provided.")
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
	coreTarget, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	if coreTarget.AccessProvider != "local" {
		return usageError("node add must run against a local Core authority Target", "Select the local Core with 'baha target activate NAME' and retry.")
	}
	verifiedTenant, err := requestNodeOwnerTenant(ctx, coreTarget.Name, opts.Environment, opts.CoreURL, opts.CAFile)
	if err != nil {
		return err
	}
	if opts.TenantID != "" && opts.TenantID != verifiedTenant {
		return machine.NewError(machine.ErrorPolicyDenied, "requested tenant does not match the verified Core operator membership", "Use the tenant assigned by the authenticated Core.", false)
	}
	opts.TenantID = verifiedTenant
	if err := validateNodeAddOptions(opts); err != nil {
		return err
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

	grant, err := requestNodeAuthorization(ctx, coreTarget.Name, opts.Environment, opts.CoreURL, opts.CAFile,
		opts.TargetID, opts.NodeID, nodeGrantLifetime, nodeCertificateLifetime)
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
		Runtime: opts.Runtime, Environment: opts.Environment, CoreTarget: coreTarget.Name, CoreURL: strings.TrimRight(opts.CoreURL, "/"),
		CoreAddress: opts.CoreAddress, ServerName: serverName, CAFile: opts.CAFile,
		// The Core grant expiry is kept only as local readiness metadata for the
		// bundle hint. node list/status never derive a trust state from it.
		GrantExpiresAt: grant.ExpiresAt,
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

func requestNodeAuthorization(ctx context.Context, coreTarget, environment, coreURL, caFile, targetID, nodeID string, grantLifetime, certificateLifetime time.Duration) (targetenrollment.Bootstrap, error) {
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
		"lifetime_seconds":        int64(grantLifetime / time.Second),
		"certificate_ttl_seconds": int64(certificateLifetime / time.Second),
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
		Timeout:       30 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, nil
}

func validCAPEM(data []byte) bool {
	pool := x509.NewCertPool()
	return pool.AppendCertsFromPEM(data)
}

// decodeCapabilitiesFromCoreResponse reads the optional capabilities array a
// Core status response advertises (contract_version/node/capabilities) without
// trusting any capability to be present or available locally. Only well-formed
// capability descriptors are surfaced; malformed data yields no capabilities and
// never a fabricated availability verdict.
func decodeCapabilitiesFromCoreResponse(data []byte) ([]nodeCapability, bool) {
	var envelope struct {
		Capabilities json.RawMessage `json:"capabilities"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Capabilities) == 0 {
		return nil, false
	}
	var raw []struct {
		Name      string `json:"name"`
		Available bool   `json:"available"`
		Detail    string `json:"detail,omitempty"`
	}
	if json.Unmarshal(envelope.Capabilities, &raw) != nil {
		return nil, false
	}
	capabilities := make([]nodeCapability, 0, len(raw))
	for _, entry := range raw {
		if strings.TrimSpace(entry.Name) == "" {
			return nil, true
		}
		capabilities = append(capabilities, nodeCapability{
			Name:      entry.Name,
			Available: entry.Available,
			Detail:    entry.Detail,
		})
	}
	if len(capabilities) == 0 {
		return nil, true
	}
	return capabilities, true
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
	if deployment.ValidateTargetName(positional[0]) != nil {
		return nodeDisconnectOwnerError()
	}
	return runNodeDisconnectWithImpact(ctx, positional[0], format, out)
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
