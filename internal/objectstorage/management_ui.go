package objectstorage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	managementUIConsumersFile = "management-ui-consumers.json"
	seaweedAdminPortEnv       = "BASEHARBOR_SEAWEEDFS_ADMIN_PORT"
	seaweedAdminUserEnv       = "BASEHARBOR_SEAWEEDFS_ADMIN_USER"
	seaweedAdminPasswordEnv   = "BASEHARBOR_SEAWEEDFS_ADMIN_PASSWORD"
)

type managementUIConsumers struct {
	Version   int      `json:"version"`
	Consumers []string `json:"consumers"`
}

func RegisterManagementUIConsumerAt(dataDir, namespace string, app application.Manifest) error {
	if !app.Services.ObjectStorageManagementUI {
		return nil
	}
	dir := filepath.Join(filepath.Clean(dataDir), "providers", "seaweedfs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	state, err := loadManagementUIConsumers(dir)
	if errors.Is(err, os.ErrNotExist) {
		state = managementUIConsumers{Version: 1}
	} else if err != nil {
		return err
	}
	key := app.Name + "/" + app.Environment
	for _, existing := range state.Consumers {
		if existing == key {
			return nil
		}
	}
	state.Consumers = append(state.Consumers, key)
	sort.Strings(state.Consumers)
	return saveManagementUIConsumers(dir, state)
}

func UnregisterManagementUIConsumerAt(dataDir, namespace string, app application.Manifest) error {
	if !app.Services.ObjectStorageManagementUI {
		return nil
	}
	dir := filepath.Join(filepath.Clean(dataDir), "providers", "seaweedfs")
	state, err := loadManagementUIConsumers(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	key := app.Name + "/" + app.Environment
	filtered := state.Consumers[:0]
	for _, existing := range state.Consumers {
		if existing != key {
			filtered = append(filtered, existing)
		}
	}
	state.Consumers = filtered
	return saveManagementUIConsumers(dir, state)
}

func managementUIRequested(dir string) (bool, error) {
	state, err := loadManagementUIConsumers(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return len(state.Consumers) > 0, nil
}

func loadManagementUIConsumers(dir string) (managementUIConsumers, error) {
	path := filepath.Join(dir, managementUIConsumersFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return managementUIConsumers{}, err
	}
	var state managementUIConsumers
	if err := json.Unmarshal(data, &state); err != nil {
		return managementUIConsumers{}, fmt.Errorf("decode SeaweedFS management UI registrations: %w", err)
	}
	if state.Version != 1 {
		return managementUIConsumers{}, fmt.Errorf("unsupported SeaweedFS management UI registration version %d", state.Version)
	}
	return state, nil
}

func saveManagementUIConsumers(dir string, state managementUIConsumers) error {
	state.Version = 1
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(dir, managementUIConsumersFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func ensureManagementUIValues(values map[string]string) error {
	if values[seaweedAdminPortEnv] == "" {
		port, err := allocatePort()
		if err != nil {
			return err
		}
		values[seaweedAdminPortEnv] = strconv.Itoa(port)
	}
	if values[seaweedAdminUserEnv] == "" {
		values[seaweedAdminUserEnv] = "baseharbor"
	}
	if values[seaweedAdminPasswordEnv] == "" {
		var raw [24]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return err
		}
		values[seaweedAdminPasswordEnv] = hex.EncodeToString(raw[:])
	}
	return nil
}

func seaweedAdminAccessSpec() serviceaccess.HTTPGatewaySpec {
	return serviceaccess.HTTPGatewaySpec{
		ServiceName:      "seaweedfs-admin-access",
		Upstream:         "http://seaweedfs-admin:23646",
		PublishedPortEnv: seaweedAdminPortEnv,
		ContainerPort:    9443,
		Networks:         []string{"object-storage"},
		RequireClient:    false,
	}
}

func providerComposeWithManagementUI(base string, access serviceaccess.HTTPGatewayFiles) string {
	service := fmt.Sprintf(`  seaweedfs-admin:
    image: %s
    restart: unless-stopped
    user: "seaweed"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    command:
      - admin
      - -ip=0.0.0.0
      - -master=seaweedfs:9333
      - -dataDir=/data
      - -iceberg.port=0
      - -lance.port=0
    environment:
      WEED_ADMIN_USER: ${%s}
      WEED_ADMIN_PASSWORD: ${%s}
    volumes:
      - seaweedfs-admin-data:/data
    networks:
      - object-storage

`, ProviderImage, seaweedAdminUserEnv, seaweedAdminPasswordEnv)
	gateway := serviceaccess.HTTPGatewayComposeService(access, seaweedAdminAccessSpec())
	marker := "volumes:\n  seaweedfs-data:\n"
	replacement := service + gateway + "volumes:\n  seaweedfs-data:\n  seaweedfs-admin-data:\n"
	if !strings.Contains(base, marker) {
		return base
	}
	return strings.Replace(base, marker, replacement, 1)
}

func ApplyDevelopmentManagementUICredentialsAt(ctx context.Context, runtime Runtime, dataDir, namespace, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return errors.New("SeaweedFS development management UI credentials are incomplete")
	}
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return err
	}
	values, err := readProviderValues(files.Env)
	if err != nil {
		return err
	}
	values[seaweedAdminUserEnv] = username
	values[seaweedAdminPasswordEnv] = password
	if err := writeEnv(files.Env, values); err != nil {
		return err
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate SeaweedFS developer management access: %w", err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("apply SeaweedFS developer management access: %w", err)
	}
	return nil
}

func ManagementUISurfaceAt(dataDir, namespace string) (application.ManagementUISurface, error) {
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return application.ManagementUISurface{}, err
	}
	values, err := readProviderValues(files.Env)
	if err != nil {
		return application.ManagementUISurface{}, err
	}
	port, err := strconv.Atoi(values[seaweedAdminPortEnv])
	if err != nil || port < 1 || port > 65535 {
		return application.ManagementUISurface{}, errors.New("SeaweedFS management UI is not materialized")
	}
	return application.ManagementUISurface{
		Service: "object-storage", Purpose: application.ProviderInterfaceManagement,
		URL:            "https://127.0.0.1:" + strconv.Itoa(port) + "/",
		Authentication: "seaweedfs-native",
	}, nil
}

func VerifyManagementUIAt(ctx context.Context, dataDir, namespace string) error {
	files, err := ExistingProviderFilesAt(dataDir, namespace)
	if err != nil {
		return err
	}
	values, err := readProviderValues(files.Env)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(values[seaweedAdminPortEnv])
	if err != nil || port < 1 || port > 65535 {
		return errors.New("SeaweedFS management UI port is invalid")
	}
	policy, err := serviceaccess.Resolve("prod", "seaweedfs-admin", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "management-ui", "service-access", "pki"))
	if err != nil {
		return err
	}
	client, err := serviceaccess.NewHTTPClient(material, false)
	if err != nil {
		return err
	}
	endpoint := "https://127.0.0.1:" + strconv.Itoa(port)
	return serviceaccess.WaitHTTPS(ctx, client, endpoint, "/")
}

func readProviderValues(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseEnv(data)
}
