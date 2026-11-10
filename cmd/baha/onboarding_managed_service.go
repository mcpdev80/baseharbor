package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

var managedCoreServiceTargetPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// ensureManagedCoreLANService provisions a repeatable, OS-supervised, per-user
// Core HTTPS listener from the installed baha binary. It never forks an
// unmanaged daemon, exposes an unauthenticated operator route, or alters a
// pre-existing externally configured serving service. The user owns both the
// unit and its private certificate material.
func ensureManagedCoreLANService(ctx context.Context, target string) error {
	if runtime.GOOS != "linux" {
		return errors.New("automatic managed Core LAN service requires Linux systemd user services")
	}
	if !managedCoreServiceTargetPattern.MatchString(target) {
		return errors.New("selected Core target is not safe for a managed service unit")
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return err
	}
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() {
		return errors.New("installed BaseHarbor CLI executable is unavailable")
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		configHome = filepath.Join(home, ".config")
	}
	unitDir := filepath.Join(configHome, "systemd", "user")
	if err := os.MkdirAll(unitDir, 0o700); err != nil {
		return err
	}
	unitName := "baseharbor-core-" + target + ".service"
	unitPath := filepath.Join(unitDir, unitName)
	environment := []string{"BASEHARBOR_TARGET=" + target}
	// Preserve only whitelisted runtime selectors; never copy the process
	// environment, bearer tokens or password files into a persistent unit.
	for _, name := range []string{"BASEHARBOR_STATE_DIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_RUNTIME_DIR", "DOCKER_HOST", "CONTAINER_HOST"} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	var unit strings.Builder
	unit.WriteString("[Unit]\nDescription=BaseHarbor owned Core LAN HTTPS (" + target + ")\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\n")
	for _, item := range environment {
		unit.WriteString("Environment=" + strconv.Quote(item) + "\n")
	}
	unit.WriteString("ExecStart=" + strconv.Quote(binary) + " serve\n")
	unit.WriteString("Restart=on-failure\nRestartSec=3\n")
	unit.WriteString("UMask=0077\nNoNewPrivileges=true\n")
	unit.WriteString("\n[Install]\nWantedBy=default.target\n")
	// Reconfiguration is always explicit and local: refuse to follow a
	// symlinked unit or overwrite a unit not carrying our managed signature.
	if original, err := os.Lstat(unitPath); err == nil {
		if !original.Mode().IsRegular() || original.Mode().Perm()&0o022 != 0 {
			return errors.New("managed Core unit is not an owner-private regular file")
		}
		payload, err := os.ReadFile(unitPath)
		if err != nil {
			return err
		}
		if !strings.Contains(string(payload), "Description=BaseHarbor owned Core LAN HTTPS (") {
			return errors.New("systemd unit name belongs to another service")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := replaceManagedCoreTLSFile(unitPath, []byte(unit.String()), 0o600); err != nil {
		return err
	}
	invoke := func(arguments ...string) error {
		cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, arguments...)...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemd user service %v failed: %w (%s)", arguments, err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	if err := invoke("daemon-reload"); err != nil {
		return err
	}
	if err := invoke("enable", "--now", unitName); err != nil {
		return err
	}
	// A unit can be running while the Core authority is still starting.
	// Check a TLS-pinned readiness probe, never just an open TCP socket.
	deadline := time.Now().Add(35 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = verifyManagedCoreLANReady(ctx, target)
		if lastErr == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(350 * time.Millisecond):
		}
	}
	return fmt.Errorf("managed Core LAN service failed pinned HTTPS readiness: %w", lastErr)
}

func verifyManagedCoreLANReady(ctx context.Context, target string) error {
	origin, err := loadOwnedCoreLANEndpoint(target)
	if err != nil {
		return err
	}
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		return err
	}
	caBytes, err := os.ReadFile(filepath.Join(root, "managed-serving", "core-api", "ca.pem"))
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caBytes) {
		return errors.New("managed Core LAN CA is invalid")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport:     transport,
		Timeout:       2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/readyz", nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Core LAN HTTPS readiness returned HTTP %d", res.StatusCode)
	}
	return nil
}
