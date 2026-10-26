package providerui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	PGAdminImage      = "docker.io/dpage/pgadmin4:9.18"
	RedisInsightImage = "docker.io/redis/redisinsight:3.8.0"
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
	DestroyProject(context.Context, string, string, string) error
}

type Endpoint struct {
	Name  string `json:"name"`
	Class string `json:"class"`
	URL   string `json:"url,omitempty"`
	Scope string `json:"scope"`
}

type ApplicationState struct {
	Version     int        `json:"version"`
	Application string     `json:"application"`
	Environment string     `json:"environment"`
	Endpoints   []Endpoint `json:"endpoints,omitempty"`
}

type ApplicationFiles struct {
	Dir     string
	Compose string
	Env     string
	State   string
	Project string
}

func EnsureApplication(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, m application.Manifest, files application.RuntimeFiles) (ApplicationState, error) {
	if runtime == nil {
		return ApplicationState{}, errors.New("provider UI runtime is required")
	}
	wantPostgres := m.Services.SQLManagementUI
	wantCache := m.Services.CacheManagementUI || m.Services.KeyValueManagementUI
	if !wantPostgres && !wantCache {
		return ApplicationState{Version: 1, Application: m.Name, Environment: m.Environment}, nil
	}
	ui := applicationFiles(m, files)
	if err := os.MkdirAll(ui.Dir, 0o700); err != nil {
		return ApplicationState{}, err
	}
	values, err := readEnv(ui.Env)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ApplicationState{}, err
	}
	if values == nil {
		values = map[string]string{}
	}
	if values["BASEHARBOR_UI_ADMIN_PASSWORD"] == "" {
		secret, err := randomSecret(24)
		if err != nil {
			return ApplicationState{}, err
		}
		values["BASEHARBOR_UI_ADMIN_PASSWORD"] = secret
	}
	if wantPostgres && values["BASEHARBOR_PGADMIN_PORT"] == "" && isDev(m.Environment) {
		port, err := allocatePort(nil)
		if err != nil {
			return ApplicationState{}, err
		}
		values["BASEHARBOR_PGADMIN_PORT"] = strconv.Itoa(port)
	}
	exclude := map[int]struct{}{}
	if n, _ := strconv.Atoi(values["BASEHARBOR_PGADMIN_PORT"]); n > 0 {
		exclude[n] = struct{}{}
	}
	if wantCache && values["BASEHARBOR_REDISINSIGHT_PORT"] == "" && isDev(m.Environment) {
		port, err := allocatePort(exclude)
		if err != nil {
			return ApplicationState{}, err
		}
		values["BASEHARBOR_REDISINSIGHT_PORT"] = strconv.Itoa(port)
	}
	if err := writeEnv(ui.Env, values); err != nil {
		return ApplicationState{}, err
	}

	bindings := application.WorkloadServiceBindingProjectionDir(files)
	var postgresConfigs []postgresBinding
	var cacheConfigs []cacheBinding
	if wantPostgres {
		postgresConfigs, err = loadPostgresBindings(bindings, ui.Dir)
		if err != nil {
			return ApplicationState{}, err
		}
		if len(postgresConfigs) == 0 {
			return ApplicationState{}, errors.New("PostgreSQL management UI requested but no PostgreSQL binding exists")
		}
	}
	if wantCache {
		cacheConfigs, err = loadCacheBindings(bindings, ui.Dir)
		if err != nil {
			return ApplicationState{}, err
		}
		if len(cacheConfigs) == 0 {
			return ApplicationState{}, errors.New("cache management UI requested but no cache binding exists")
		}
	}

	var pgAccess, redisAccess serviceaccess.HTTPGatewayFiles
	var pgSpec, redisSpec serviceaccess.HTTPGatewaySpec
	state := ApplicationState{Version: 1, Application: m.Name, Environment: m.Environment}
	if wantPostgres {
		policy, err := serviceaccess.Resolve(m.Environment, "pgadmin-ui", serviceaccess.AuthenticationNative)
		if err != nil {
			return ApplicationState{}, err
		}
		published := ""
		if isDev(m.Environment) {
			published = "BASEHARBOR_PGADMIN_PORT"
		}
		pgSpec = serviceaccess.HTTPGatewaySpec{
			ServiceName: "pgadmin-ui-access", Upstream: "http://pgadmin:8080",
			PublishedPortEnv: published, ContainerPort: 9443, Networks: []string{"backend"},
		}
		pgAccess, err = serviceaccess.EnsureHTTPGateway(ctx, issuer, policy, filepath.Join(ui.Dir, "pgadmin-access"), pgSpec)
		if err != nil {
			return ApplicationState{}, err
		}
		endpoint := Endpoint{Name: "postgresql-management", Class: "administration", Scope: "application"}
		if isDev(m.Environment) {
			endpoint.URL = "https://127.0.0.1:" + values["BASEHARBOR_PGADMIN_PORT"]
		}
		state.Endpoints = append(state.Endpoints, endpoint)
	}
	if wantCache {
		policy, err := serviceaccess.Resolve(m.Environment, "redisinsight-ui", serviceaccess.AuthenticationNone)
		if err != nil {
			// Test/prod deliberately do not publish the UI. Native service
			// authentication is not claimed where Redis Insight has none.
			if !isDev(m.Environment) {
				policy, err = serviceaccess.Resolve(m.Environment, "redisinsight-ui", serviceaccess.AuthenticationNative)
			}
		}
		if err != nil {
			return ApplicationState{}, err
		}
		published := ""
		if isDev(m.Environment) {
			published = "BASEHARBOR_REDISINSIGHT_PORT"
		}
		redisSpec = serviceaccess.HTTPGatewaySpec{
			ServiceName: "redisinsight-ui-access", Upstream: "http://redisinsight:5540",
			PublishedPortEnv: published, ContainerPort: 9444, Networks: []string{"backend"},
		}
		redisAccess, err = serviceaccess.EnsureHTTPGateway(ctx, issuer, policy, filepath.Join(ui.Dir, "redisinsight-access"), redisSpec)
		if err != nil {
			return ApplicationState{}, err
		}
		endpoint := Endpoint{Name: "cache-management", Class: "administration", Scope: "application"}
		if isDev(m.Environment) {
			endpoint.URL = "https://127.0.0.1:" + values["BASEHARBOR_REDISINSIGHT_PORT"]
		}
		state.Endpoints = append(state.Endpoints, endpoint)
	}

	compose, err := applicationCompose(m, files, ui, values, postgresConfigs, cacheConfigs, pgAccess, pgSpec, redisAccess, redisSpec)
	if err != nil {
		return ApplicationState{}, err
	}
	if err := os.WriteFile(ui.Compose, []byte(compose), 0o600); err != nil {
		return ApplicationState{}, err
	}
	if err := os.Chmod(ui.Compose, 0o600); err != nil {
		return ApplicationState{}, err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return ApplicationState{}, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(ui.State, data, 0o600); err != nil {
		return ApplicationState{}, err
	}
	if err := runtime.ConfigProject(ctx, ui.Project, ui.Compose, ui.Env); err != nil {
		return ApplicationState{}, fmt.Errorf("validate application provider UI project: %w", err)
	}
	if err := runtime.UpProject(ctx, ui.Project, ui.Compose, ui.Env); err != nil {
		return ApplicationState{}, fmt.Errorf("start application provider UI project: %w", err)
	}
	return state, nil
}

func DestroyApplication(ctx context.Context, runtime Runtime, m application.Manifest, files application.RuntimeFiles) error {
	ui := applicationFiles(m, files)
	if _, err := os.Stat(ui.Compose); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := runtime.DestroyProject(ctx, ui.Project, ui.Compose, ui.Env); err != nil {
		return err
	}
	return os.RemoveAll(ui.Dir)
}

func ExistingApplicationState(m application.Manifest, files application.RuntimeFiles) (ApplicationState, error) {
	ui := applicationFiles(m, files)
	data, err := os.ReadFile(ui.State)
	if err != nil {
		return ApplicationState{}, err
	}
	var state ApplicationState
	if err := json.Unmarshal(data, &state); err != nil {
		return ApplicationState{}, err
	}
	return state, nil
}

type postgresBinding struct {
	Name, Host, Port, Database, Username, Password, CAPath string
}
type cacheBinding struct {
	Name, Host, Port, Password, CAPath string
}

func loadPostgresBindings(root, uiDir string) ([]postgresBinding, error) {
	entries, err := readBindings(root, "postgresql")
	if err != nil {
		return nil, err
	}
	var result []postgresBinding
	caDir := filepath.Join(uiDir, "pgadmin-ca")
	if err := os.MkdirAll(caDir, 0o700); err != nil {
		return nil, err
	}
	for i, entry := range entries {
		ca := filepath.Join(caDir, fmt.Sprintf("%d.pem", i+1))
		if err := copyBindingEntry(entry.Dir, "certificates", ca, 0o644); err != nil {
			return nil, err
		}
		result = append(result, postgresBinding{
			Name: entry.Name, Host: entry.Values["host"], Port: entry.Values["port"],
			Database: entry.Values["database"], Username: entry.Values["username"],
			Password: entry.Values["password"], CAPath: ca,
		})
	}
	return result, nil
}

func loadCacheBindings(root, uiDir string) ([]cacheBinding, error) {
	entries, err := readBindings(root, "redis")
	if err != nil {
		return nil, err
	}
	var result []cacheBinding
	caDir := filepath.Join(uiDir, "redisinsight-ca")
	if err := os.MkdirAll(caDir, 0o700); err != nil {
		return nil, err
	}
	for i, entry := range entries {
		ca := filepath.Join(caDir, fmt.Sprintf("%d.pem", i+1))
		if err := copyBindingEntry(entry.Dir, "certificates", ca, 0o644); err != nil {
			return nil, err
		}
		result = append(result, cacheBinding{Name: entry.Name, Host: entry.Values["host"], Port: entry.Values["port"], Password: entry.Values["password"], CAPath: ca})
	}
	return result, nil
}

type bindingEntry struct {
	Name, Dir string
	Values    map[string]string
}

func readBindings(root, typ string) ([]bindingEntry, error) {
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var result []bindingEntry
	for _, item := range dirs {
		if !item.IsDir() {
			continue
		}
		dir := filepath.Join(root, item.Name())
		typeData, err := os.ReadFile(filepath.Join(dir, "type"))
		if err != nil || strings.TrimSpace(string(typeData)) != typ {
			continue
		}
		values := map[string]string{}
		for _, name := range []string{"host", "port", "database", "username", "password"} {
			if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
				values[name] = strings.TrimSpace(string(data))
			}
		}
		if values["host"] == "" || values["port"] == "" {
			return nil, fmt.Errorf("binding %s is incomplete", item.Name())
		}
		result = append(result, bindingEntry{Name: item.Name(), Dir: dir, Values: values})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func applicationCompose(m application.Manifest, files application.RuntimeFiles, ui ApplicationFiles, env map[string]string, postgres []postgresBinding, cache []cacheBinding, pgAccess serviceaccess.HTTPGatewayFiles, pgSpec serviceaccess.HTTPGatewaySpec, redisAccess serviceaccess.HTTPGatewayFiles, redisSpec serviceaccess.HTTPGatewaySpec) (string, error) {
	var b strings.Builder
	b.WriteString("services:\n")
	if len(postgres) > 0 {
		servers := map[string]any{"Servers": map[string]any{}}
		serverMap := servers["Servers"].(map[string]any)
		var pass strings.Builder
		for i, cfg := range postgres {
			key := strconv.Itoa(i + 1)
			serverMap[key] = map[string]any{
				"Name": cfg.Name, "Group": "BaseHarbor", "Host": cfg.Host,
				"Port": mustPort(cfg.Port), "MaintenanceDB": cfg.Database, "Username": cfg.Username,
				"ConnectionParameters": map[string]any{"sslmode": "verify-ca", "sslrootcert": "/etc/baseharbor/pg-ca/" + filepath.Base(cfg.CAPath), "passfile": "/etc/baseharbor/pgpass", "connect_timeout": 10},
			}
			fmt.Fprintf(&pass, "%s:%s:%s:%s:%s\n", pgpassEscape(cfg.Host), pgpassEscape(cfg.Port), pgpassEscape(cfg.Database), pgpassEscape(cfg.Username), pgpassEscape(cfg.Password))
		}
		data, _ := json.MarshalIndent(servers, "", "  ")
		serversPath := filepath.Join(ui.Dir, "pgadmin-servers.json")
		passPath := filepath.Join(ui.Dir, "pgpass")
		if err := os.WriteFile(serversPath, append(data, '\n'), 0o600); err != nil {
			return "", err
		}
		if err := os.WriteFile(passPath, []byte(pass.String()), 0o600); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, `  pgadmin:
    image: %s
    restart: unless-stopped
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    environment:
      PGADMIN_DEFAULT_EMAIL: "baseharbor@localhost.invalid"
      PGADMIN_DEFAULT_PASSWORD: "${BASEHARBOR_UI_ADMIN_PASSWORD}"
      PGADMIN_LISTEN_PORT: "8080"
      PGADMIN_SERVER_JSON_FILE: /etc/baseharbor/servers.json
      PGADMIN_REPLACE_SERVERS_ON_STARTUP: "True"
      PGPASS_FILE: /etc/baseharbor/pgpass
      PGADMIN_CONFIG_MASTER_PASSWORD_REQUIRED: "False"
    volumes:
      - pgadmin-data:/var/lib/pgadmin
      - %q:/etc/baseharbor/servers.json:ro
      - %q:/etc/baseharbor/pgpass:ro
      - %q:/etc/baseharbor/pg-ca:ro
    networks:
      - backend
`, PGAdminImage, serversPath, passPath, filepath.Join(ui.Dir, "pgadmin-ca"))
		b.WriteString(serviceaccess.HTTPGatewayComposeService(pgAccess, pgSpec))
	}
	if len(cache) > 0 {
		fmt.Fprintf(&b, `  redisinsight:
    image: %s
    restart: unless-stopped
    user: "1000:1000"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    volumes:
      - redisinsight-data:/data
      - %q:/etc/baseharbor/redis-ca:ro
    environment:
`, RedisInsightImage, filepath.Join(ui.Dir, "redisinsight-ca"))
		for i, cfg := range cache {
			suffix := strconv.Itoa(i)
			fmt.Fprintf(&b, "      RI_REDIS_HOST%s: %q\n", suffix, cfg.Host)
			fmt.Fprintf(&b, "      RI_REDIS_PORT%s: %q\n", suffix, cfg.Port)
			fmt.Fprintf(&b, "      RI_REDIS_ALIAS%s: %q\n", suffix, cfg.Name)
			fmt.Fprintf(&b, "      RI_REDIS_USERNAME%s: %q\n", suffix, "default")
			fmt.Fprintf(&b, "      RI_REDIS_PASSWORD%s: %q\n", suffix, cfg.Password)
			fmt.Fprintf(&b, "      RI_REDIS_TLS%s: %q\n", suffix, "true")
			fmt.Fprintf(&b, "      RI_REDIS_TLS_CA_PATH%s: %q\n", suffix, "/etc/baseharbor/redis-ca/"+filepath.Base(cfg.CAPath))
		}
		b.WriteString("    networks:\n      - backend\n")
		b.WriteString(serviceaccess.HTTPGatewayComposeService(redisAccess, redisSpec))
	}
	b.WriteString("volumes:\n")
	if len(postgres) > 0 {
		b.WriteString("  pgadmin-data:\n")
	}
	if len(cache) > 0 {
		b.WriteString("  redisinsight-data:\n")
	}
	b.WriteString("networks:\n  backend:\n    external: true\n")
	fmt.Fprintf(&b, "    name: %q\n", application.ApplicationBackendNetworkNameForProject(files.ResourceProject))
	return b.String(), nil
}

func applicationFiles(m application.Manifest, files application.RuntimeFiles) ApplicationFiles {
	dir := filepath.Join(files.Dir, "provider-ui")
	return ApplicationFiles{
		Dir: dir, Compose: filepath.Join(dir, "compose.yaml"), Env: filepath.Join(dir, "runtime.env"), State: filepath.Join(dir, "state.json"),
		Project: bhruntime.ApplicationProjectName(files.Namespace, m.Name+"-ui", m.Environment),
	}
}

func copyBindingEntry(dir, name, target string, mode os.FileMode) error {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, data, mode); err != nil {
		return err
	}
	return os.Chmod(target, mode)
}
func mustPort(raw string) int { n, _ := strconv.Atoi(raw); return n }
func pgpassEscape(v string) string {
	v = strings.ReplaceAll(v, "\\", "\\\\")
	return strings.ReplaceAll(v, ":", "\\:")
}
func isDev(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "dev", "development":
		return true
	}
	return false
}
func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func allocatePort(exclude map[int]struct{}) (int, error) {
	for i := 0; i < 16; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		p := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if _, ok := exclude[p]; !ok {
			return p, nil
		}
	}
	return 0, errors.New("allocate provider UI port")
}
func readEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, errors.New("invalid provider UI environment")
		}
		m[k] = v
	}
	return m, nil
}
func writeEnv(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, values[k])
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
