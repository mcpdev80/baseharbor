package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func openBaoManagementUISurface(files bhruntime.Files) (application.ManagementUISurface, error) {
	port, err := openBaoPublishedPort(files)
	if err != nil {
		return application.ManagementUISurface{}, err
	}
	endpoint, err := serviceaccess.LoopbackHTTPSURL(port)
	if err != nil {
		return application.ManagementUISurface{}, err
	}
	return application.ManagementUISurface{
		Service:             "secrets",
		Purpose:             application.ProviderInterfaceAdministration,
		URL:                 endpoint + "/ui/",
		Authentication:      "openbao-native",
		AuthenticationClass: application.ManagementAuthNativeCredential,
		RoleMappings:        application.NativeCredentialRoleMappings("root/admin", "default"),
	}, nil
}

func verifyOpenBaoManagementUI(ctx context.Context, files bhruntime.Files) error {
	port, err := openBaoPublishedPort(files)
	if err != nil {
		return err
	}
	policy, err := serviceaccess.Resolve("prod", "openbao", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	root := filepath.Join(filepath.Dir(files.Compose), "providers", "openbao", "service-access", "pki")
	material, err := serviceaccess.ExistingTLSMaterial(policy, root)
	if err != nil {
		return fmt.Errorf("inspect OpenBao management UI TLS: %w", err)
	}
	client, err := serviceaccess.NewHTTPClient(material, false)
	if err != nil {
		return err
	}
	endpoint, err := serviceaccess.LoopbackHTTPSURL(port)
	if err != nil {
		return err
	}
	if err := serviceaccess.WaitHTTPS(ctx, client, endpoint, "/ui/"); err != nil {
		return fmt.Errorf("OpenBao management UI is not ready: %w", err)
	}
	return nil
}

func openBaoPublishedPort(files bhruntime.Files) (int, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return 0, err
	}
	value := ""
	for _, line := range strings.Split(string(data), "\n") {
		key, raw, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "BASEHARBOR_OPENBAO_PORT" {
			value = strings.TrimSpace(raw)
			break
		}
	}
	if value == "" {
		return 0, errors.New("OpenBao management UI port is not materialized")
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("OpenBao management UI port is invalid")
	}
	return port, nil
}
