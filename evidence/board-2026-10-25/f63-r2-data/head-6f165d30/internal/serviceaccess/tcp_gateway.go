package serviceaccess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const TCPGatewayImage = "docker.io/library/haproxy:3.2.23-alpine"

type TCPGatewayFiles struct {
	Dir      string
	Config   string
	PEM      string
	Material TLSMaterial
}

type TCPGatewayUpstream struct {
	Name string
	Host string
	Port int
}

type TCPGatewaySpec struct {
	ServiceName       string
	UpstreamHost      string
	UpstreamPort      int
	Upstreams         []TCPGatewayUpstream
	PublishedPortEnv  string
	ContainerPort     int
	Network           string
	Environment       map[string]string
	BackendDirectives []string
	ServerDirectives  []string
}

func EnsureTCPGateway(ctx context.Context, issuer Issuer, policy Policy, providerDir string, spec TCPGatewaySpec) (TCPGatewayFiles, error) {
	if strings.TrimSpace(spec.ServiceName) == "" {
		return TCPGatewayFiles{}, errors.New("TCP service gateway name is required")
	}
	upstreams, err := normalizeTCPGatewayUpstreams(spec)
	if err != nil {
		return TCPGatewayFiles{}, err
	}
	spec.Upstreams = upstreams
	if spec.ContainerPort == 0 {
		spec.ContainerPort = spec.UpstreamPort
	}
	if spec.ContainerPort < 1 || spec.ContainerPort > 65535 {
		return TCPGatewayFiles{}, errors.New("TCP service gateway container port is invalid")
	}
	dir := filepath.Join(providerDir, "service-access")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return TCPGatewayFiles{}, err
	}
	material, err := EnsureTLSMaterial(ctx, issuer, policy, filepath.Join(dir, "pki"), spec.ServiceName, "127.0.0.1")
	if err != nil {
		return TCPGatewayFiles{}, err
	}
	projected, err := projectTCPMaterial(dir, material)
	if err != nil {
		return TCPGatewayFiles{}, err
	}
	files := TCPGatewayFiles{
		Dir:      dir,
		Config:   filepath.Join(dir, "haproxy.cfg"),
		PEM:      filepath.Join(dir, "runtime", "server.pem"),
		Material: projected,
	}
	cfg := tcpGatewayConfig(spec)
	if err := writeAtomic(files.Config, []byte(cfg), 0o644); err != nil {
		return TCPGatewayFiles{}, err
	}
	return files, nil
}

func projectTCPMaterial(dir string, material TLSMaterial) (TLSMaterial, error) {
	runtimeDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return TLSMaterial{}, err
	}
	cert, err := os.ReadFile(material.ServerCertificate)
	if err != nil {
		return TLSMaterial{}, err
	}
	key, err := os.ReadFile(material.ServerKey)
	if err != nil {
		return TLSMaterial{}, err
	}
	ca, err := os.ReadFile(material.CA)
	if err != nil {
		return TLSMaterial{}, err
	}
	pemPath := filepath.Join(runtimeDir, "server.pem")
	combined := append(append([]byte(nil), cert...), '\n')
	combined = append(combined, key...)
	// These files are bind-mounted into a long-running gateway. Preserve the
	// inode across certificate replacement so a graceful HAProxy reload sees
	// the new material instead of a stale pre-rename bind mount.
	if err := os.WriteFile(pemPath, combined, 0o644); err != nil {
		return TLSMaterial{}, err
	}
	if err := os.Chmod(pemPath, 0o644); err != nil {
		return TLSMaterial{}, err
	}
	caPath := filepath.Join(runtimeDir, "ca.pem")
	if err := os.WriteFile(caPath, ca, 0o644); err != nil {
		return TLSMaterial{}, err
	}
	if err := os.Chmod(caPath, 0o644); err != nil {
		return TLSMaterial{}, err
	}
	material.ServerCertificate = pemPath
	material.ServerKey = pemPath
	material.CA = caPath
	return material, nil
}

func TCPGatewayComposeService(files TCPGatewayFiles, spec TCPGatewaySpec) string {
	if spec.ContainerPort == 0 {
		spec.ContainerPort = spec.UpstreamPort
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s:\n", spec.ServiceName)
	fmt.Fprintf(&b, "    image: %s\n", TCPGatewayImage)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"99:99\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs: [\"/tmp:rw,noexec,nosuid,nodev\"]\n")
	b.WriteString("    command: [\"haproxy\", \"-W\", \"-db\", \"-f\", \"/usr/local/etc/haproxy/haproxy.cfg\"]\n")
	if len(spec.Environment) > 0 {
		keys := make([]string, 0, len(spec.Environment))
		for key := range spec.Environment {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteString("    environment:\n")
		for _, key := range keys {
			fmt.Fprintf(&b, "      %s: %s\n", key, strconv.Quote(spec.Environment[key]))
		}
	}
	if strings.TrimSpace(spec.PublishedPortEnv) != "" {
		b.WriteString("    ports:\n")
		fmt.Fprintf(&b, "      - \"127.0.0.1:$"+"{%s}:%d\"\n", spec.PublishedPortEnv, spec.ContainerPort)
	}
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Config+":/usr/local/etc/haproxy/haproxy.cfg:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.PEM+":/run/baseharbor/tls/server.pem:ro"))
	if strings.TrimSpace(spec.Network) != "" {
		b.WriteString("    networks:\n")
		alias := strings.TrimSpace(files.Material.ServerName)
		if alias != "" && !strings.EqualFold(alias, "localhost") && alias != spec.ServiceName {
			fmt.Fprintf(&b, "      %s:\n        aliases:\n          - %s\n", spec.Network, strconv.Quote(alias))
		} else {
			fmt.Fprintf(&b, "      %s: {}\n", spec.Network)
		}
	}
	return b.String()
}

func tcpGatewayConfig(spec TCPGatewaySpec) string {
	if spec.ContainerPort == 0 {
		spec.ContainerPort = spec.UpstreamPort
	}
	upstreams, err := normalizeTCPGatewayUpstreams(spec)
	if err != nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, `global
  log stdout format raw local0
  ssl-default-bind-options ssl-min-ver TLSv1.2

defaults
  mode tcp
  log global
  timeout connect 5s
  timeout client 30s
  timeout server 30s

frontend service
  bind :%d ssl crt /run/baseharbor/tls/server.pem
  default_backend upstream

listen baseharbor_stats
  bind 127.0.0.1:8404
  mode http
  stats enable
  stats uri /stats
  stats show-legends

backend upstream
  balance roundrobin
  option log-health-checks
  default-server resolvers runtime-dns resolve-prefer ipv4 init-addr last,libc,none on-marked-down shutdown-sessions
`, spec.ContainerPort)
	for _, directive := range spec.BackendDirectives {
		directive = strings.TrimSpace(directive)
		if directive == "" {
			continue
		}
		fmt.Fprintf(&b, "  %s\n", directive)
	}
	serverDirectives := make([]string, 0, len(spec.ServerDirectives))
	for _, directive := range spec.ServerDirectives {
		directive = strings.TrimSpace(directive)
		if directive != "" {
			serverDirectives = append(serverDirectives, directive)
		}
	}
	suffix := ""
	if len(serverDirectives) > 0 {
		suffix = " " + strings.Join(serverDirectives, " ")
	}
	for _, upstream := range upstreams {
		fmt.Fprintf(&b, "  server %s %s:%d check%s\n", upstream.Name, upstream.Host, upstream.Port, suffix)
	}
	b.WriteString("\nresolvers runtime-dns\n  parse-resolv-conf\n  hold valid 2s\n  hold obsolete 1s\n  hold nx 1s\n  timeout resolve 1s\n  timeout retry 1s\n")
	return b.String()
}

func normalizeTCPGatewayUpstreams(spec TCPGatewaySpec) ([]TCPGatewayUpstream, error) {
	upstreams := append([]TCPGatewayUpstream(nil), spec.Upstreams...)
	if len(upstreams) == 0 {
		if strings.TrimSpace(spec.UpstreamHost) == "" {
			return nil, errors.New("TCP service gateway upstream is required")
		}
		upstreams = []TCPGatewayUpstream{{Name: "provider", Host: spec.UpstreamHost, Port: spec.UpstreamPort}}
	}
	seen := map[string]struct{}{}
	for i := range upstreams {
		upstreams[i].Name = strings.TrimSpace(upstreams[i].Name)
		upstreams[i].Host = strings.TrimSpace(upstreams[i].Host)
		if upstreams[i].Name == "" {
			upstreams[i].Name = fmt.Sprintf("provider-%d", i+1)
		}
		if upstreams[i].Host == "" {
			return nil, errors.New("TCP service gateway upstream host is required")
		}
		if upstreams[i].Port < 1 || upstreams[i].Port > 65535 {
			return nil, errors.New("TCP service gateway upstream port is invalid")
		}
		if _, exists := seen[upstreams[i].Name]; exists {
			return nil, fmt.Errorf("TCP service gateway upstream name %q is duplicated", upstreams[i].Name)
		}
		seen[upstreams[i].Name] = struct{}{}
	}
	return upstreams, nil
}
