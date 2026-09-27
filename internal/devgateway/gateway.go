package devgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const (
	stateVersion = 1
	gatewayPort  = 443
)

type Runtime interface {
	ConfigProject(context.Context, string, string, string) error
	UpProject(context.Context, string, string, string) error
}

type Route struct {
	Owner      string `json:"owner"`
	Key        string `json:"key"`
	Host       string `json:"host"`
	Upstream   string `json:"upstream"`
	Network    string `json:"network"`
	TrustFile  string `json:"trust_file"`
	ServerName string `json:"server_name"`
}

type state struct {
	Version int     `json:"version"`
	Routes  []Route `json:"routes"`
}

type Files struct {
	Dir       string
	State     string
	Compose   string
	Env       string
	Caddyfile string
	Project   string
	Cert      string
	Key       string
	CA        string
}

func FilesFor(target string) (Files, error) {
	root, err := deployment.TargetStateRoot(strings.TrimSpace(target))
	if err != nil {
		return Files{}, err
	}
	dir := filepath.Join(root, "developer-access", "dev", "gateway")
	return Files{
		Dir:       dir,
		State:     filepath.Join(dir, "routes.json"),
		Compose:   filepath.Join(dir, "compose.yaml"),
		Env:       filepath.Join(dir, "runtime.env"),
		Caddyfile: filepath.Join(dir, "Caddyfile"),
		Project:   bhruntime.ApplicationProjectName(target, "dev-gateway", "dev"),
		Cert:      filepath.Join(dir, "runtime", "server.pem"),
		Key:       filepath.Join(dir, "runtime", "server-key.pem"),
		CA:        filepath.Join(dir, "runtime", "ca.pem"),
	}, nil
}

func ReplaceOwnerRoutes(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target, owner string, routes []Route) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return errors.New("development gateway route owner is required")
	}
	files, err := FilesFor(target)
	if err != nil {
		return err
	}
	current, err := loadState(files.State)
	if errors.Is(err, os.ErrNotExist) {
		current = state{Version: stateVersion}
	} else if err != nil {
		return err
	}
	filtered := current.Routes[:0]
	for _, route := range current.Routes {
		if route.Owner != owner {
			filtered = append(filtered, route)
		}
	}
	for _, route := range routes {
		route.Owner = owner
		if err := validateRoute(route); err != nil {
			return err
		}
		filtered = append(filtered, route)
	}
	current.Routes = normalizedRoutes(filtered)
	if err := saveState(files.State, current); err != nil {
		return err
	}
	return Reconcile(ctx, runtime, issuer, target)
}

func Reconcile(ctx context.Context, runtime Runtime, issuer serviceaccess.Issuer, target string) error {
	if runtime == nil {
		return errors.New("development gateway runtime is required")
	}
	if issuer == nil {
		return errors.New("development gateway issuer is required")
	}
	files, err := FilesFor(target)
	if err != nil {
		return err
	}
	current, err := loadState(files.State)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(current.Routes) == 0 {
		return nil
	}
	if _, err := os.Stat(files.Compose); errors.Is(err, os.ErrNotExist) {
		if err := checkGatewayPort(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(files.Dir, "runtime", "trust"), 0o700); err != nil {
		return err
	}
	hosts := make([]string, 0, len(current.Routes))
	for _, route := range current.Routes {
		hosts = append(hosts, route.Host)
	}
	policy, err := serviceaccess.Resolve("dev", "dev-gateway", serviceaccess.AuthenticationNative)
	if err != nil {
		return err
	}
	policy.ServerName = hosts[0]
	material, err := serviceaccess.EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(files.Dir, "pki"), hosts...)
	if err != nil {
		return fmt.Errorf("prepare development gateway TLS: %w", err)
	}
	if err := projectReadable(material.CA, files.CA); err != nil {
		return err
	}
	if err := projectReadable(material.ServerCertificate, files.Cert); err != nil {
		return err
	}
	if err := projectReadable(material.ServerKey, files.Key); err != nil {
		return err
	}
	trustTargets := map[string]string{}
	for i, route := range current.Routes {
		targetPath := filepath.Join(files.Dir, "runtime", "trust", fmt.Sprintf("route-%03d.pem", i))
		if err := projectReadable(route.TrustFile, targetPath); err != nil {
			return fmt.Errorf("project trust for %s: %w", route.Key, err)
		}
		trustTargets[route.Key] = targetPath
	}
	if err := os.WriteFile(files.Caddyfile, []byte(renderCaddyfile(current.Routes)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(files.Env, []byte(""), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(files.Compose, []byte(renderCompose(files, current.Routes, trustTargets)), 0o600); err != nil {
		return err
	}
	if err := runtime.ConfigProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("validate development gateway: %w", err)
	}
	if err := runtime.UpProject(ctx, files.Project, files.Compose, files.Env); err != nil {
		return fmt.Errorf("start development gateway: %w", err)
	}
	return nil
}

func URL(host string) string {
	return "https://" + strings.TrimSpace(host)
}

func loadState(path string) (state, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return state{}, err
	}
	var value state
	if err := json.Unmarshal(data, &value); err != nil {
		return state{}, fmt.Errorf("decode development gateway routes: %w", err)
	}
	if value.Version != stateVersion {
		return state{}, fmt.Errorf("unsupported development gateway route state version %d", value.Version)
	}
	value.Routes = normalizedRoutes(value.Routes)
	return value, nil
}

func saveState(path string, value state) error {
	value.Version = stateVersion
	value.Routes = normalizedRoutes(value.Routes)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
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

func validateRoute(route Route) error {
	for label, value := range map[string]string{
		"key": route.Key, "host": route.Host, "upstream": route.Upstream,
		"network": route.Network, "trust file": route.TrustFile, "server name": route.ServerName,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("development gateway route %s is required", label)
		}
	}
	if !strings.HasPrefix(route.Upstream, "https://") {
		return fmt.Errorf("development gateway route %s must use an HTTPS upstream", route.Key)
	}
	if strings.ContainsAny(route.Host, "/:@ \t\r\n") {
		return fmt.Errorf("development gateway host %q is invalid", route.Host)
	}
	return nil
}

func normalizedRoutes(routes []Route) []Route {
	byKey := map[string]Route{}
	for _, route := range routes {
		route.Owner = strings.TrimSpace(route.Owner)
		route.Key = strings.TrimSpace(route.Key)
		route.Host = strings.ToLower(strings.TrimSpace(route.Host))
		route.Upstream = strings.TrimSpace(route.Upstream)
		route.Network = strings.TrimSpace(route.Network)
		route.TrustFile = strings.TrimSpace(route.TrustFile)
		route.ServerName = strings.TrimSpace(route.ServerName)
		if route.Key != "" {
			byKey[route.Key] = route
		}
	}
	out := make([]Route, 0, len(byKey))
	for _, route := range byKey {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Host != out[j].Host {
			return out[i].Host < out[j].Host
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func renderCaddyfile(routes []Route) string {
	var b strings.Builder
	b.WriteString("{\n  auto_https off\n}\n\n:8443 {\n  tls /certs/server.pem /certs/server-key.pem\n")
	for i, route := range routes {
		fmt.Fprintf(&b, "  @route%d host %s\n", i, route.Host)
		fmt.Fprintf(&b, "  handle @route%d {\n", i)
		fmt.Fprintf(&b, "    reverse_proxy %s {\n", route.Upstream)
		b.WriteString("      transport http {\n        tls\n")
		fmt.Fprintf(&b, "        tls_trust_pool file /trust/route-%03d.pem\n", i)
		fmt.Fprintf(&b, "        tls_server_name %s\n", route.ServerName)
		b.WriteString("      }\n    }\n  }\n")
	}
	b.WriteString("  respond 404\n}\n")
	return b.String()
}

func renderCompose(files Files, routes []Route, trustTargets map[string]string) string {
	networks := map[string]string{}
	routeNetwork := map[string]string{}
	for _, route := range routes {
		if logical, ok := networks[route.Network]; ok {
			routeNetwork[route.Key] = logical
			continue
		}
		logical := "route" + strconv.Itoa(len(networks))
		networks[route.Network] = logical
		routeNetwork[route.Key] = logical
	}
	var b strings.Builder
	b.WriteString("services:\n  dev-gateway:\n")
	b.WriteString("    image: docker.io/library/caddy:2.11.4-alpine\n")
	b.WriteString("    restart: unless-stopped\n    user: \"65532:65532\"\n    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n      - /data:rw,noexec,nosuid,nodev\n      - /config:rw,noexec,nosuid,nodev\n      - /run/baseharbor:rw,exec,nosuid,nodev,mode=1777\n")
	b.WriteString("    entrypoint: [\"/bin/sh\", \"-ec\"]\n")
	b.WriteString("    command:\n      - cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile\n")
	b.WriteString("    ports:\n      - \"127.0.0.1:443:8443\"\n")
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %q\n", files.Caddyfile+":/etc/caddy/Caddyfile:ro")
	fmt.Fprintf(&b, "      - %q\n", files.Cert+":/certs/server.pem:ro")
	fmt.Fprintf(&b, "      - %q\n", files.Key+":/certs/server-key.pem:ro")
	for _, route := range routes {
		fmt.Fprintf(&b, "      - %q\n", trustTargets[route.Key]+":/trust/"+trustMountName(routes, route.Key)+":ro")
	}
	b.WriteString("    networks:\n")
	seen := map[string]bool{}
	for _, route := range routes {
		logical := routeNetwork[route.Key]
		if seen[logical] {
			continue
		}
		seen[logical] = true
		fmt.Fprintf(&b, "      %s: {}\n", logical)
	}
	b.WriteString("\nnetworks:\n")
	actuals := make([]string, 0, len(networks))
	for actual := range networks {
		actuals = append(actuals, actual)
	}
	sort.Strings(actuals)
	for _, actual := range actuals {
		logical := networks[actual]
		fmt.Fprintf(&b, "  %s:\n    external: true\n    name: %q\n", logical, actual)
	}
	return b.String()
}

func trustMountName(routes []Route, key string) string {
	for i, route := range routes {
		if route.Key == key {
			return fmt.Sprintf("route-%03d.pem", i)
		}
	}
	return "missing.pem"
}

func projectReadable(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("%s is empty", filepath.Base(source))
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func checkGatewayPort() error {
	listener, err := net.Listen("tcp", "127.0.0.1:443")
	if err != nil {
		return fmt.Errorf("canonical development URLs require local HTTPS port 443: %w", err)
	}
	return listener.Close()
}
