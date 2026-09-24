package serviceaccess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

type TCPGatewaySpec struct {
	ServiceName      string
	UpstreamHost     string
	UpstreamPort     int
	PublishedPortEnv string
	ContainerPort    int
	Network          string
}

func EnsureTCPGateway(policy Policy, providerDir string, spec TCPGatewaySpec) (TCPGatewayFiles, error) {
	if strings.TrimSpace(spec.ServiceName) == "" || strings.TrimSpace(spec.UpstreamHost) == "" {
		return TCPGatewayFiles{}, errors.New("TCP service gateway name and upstream are required")
	}
	if spec.UpstreamPort < 1 || spec.UpstreamPort > 65535 {
		return TCPGatewayFiles{}, errors.New("TCP service gateway upstream port is invalid")
	}
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
	material, err := EnsureTLSMaterial(policy, filepath.Join(dir, "pki"), spec.ServiceName, "127.0.0.1")
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
	if err := writeAtomic(pemPath, combined, 0o644); err != nil {
		return TLSMaterial{}, err
	}
	caPath := filepath.Join(runtimeDir, "ca.pem")
	if err := writeAtomic(caPath, ca, 0o644); err != nil {
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
	return fmt.Sprintf(`global
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

backend upstream
  server provider %s:%d check
`, spec.ContainerPort, spec.UpstreamHost, spec.UpstreamPort)
}
