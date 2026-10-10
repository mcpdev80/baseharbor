package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/targetenrollment"
)

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
