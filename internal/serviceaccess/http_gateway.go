package serviceaccess

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const GatewayImage = "docker.io/library/caddy:2.11.4-alpine"

type HTTPGatewayFiles struct {
	Dir       string
	Caddyfile string
	Material  TLSMaterial
}

type HTTPGatewaySpec struct {
	ServiceName      string
	Upstream         string
	PublishedPortEnv string
	ContainerPort    int
	Networks         []string
	RequireClient    bool
}

func EnsureHTTPGateway(policy Policy, providerDir string, spec HTTPGatewaySpec) (HTTPGatewayFiles, error) {
	if strings.TrimSpace(spec.ServiceName) == "" {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway name is required")
	}
	if strings.TrimSpace(spec.Upstream) == "" {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway upstream is required")
	}
	if strings.TrimSpace(spec.PublishedPortEnv) == "" {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway published port environment is required")
	}
	if spec.ContainerPort == 0 {
		spec.ContainerPort = 8443
	}
	if spec.ContainerPort < 1 || spec.ContainerPort > 65535 {
		return HTTPGatewayFiles{}, errors.New("HTTP service gateway container port is invalid")
	}
	dir := filepath.Join(providerDir, "service-access")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return HTTPGatewayFiles{}, fmt.Errorf("create service access state: %w", err)
	}
	material, err := EnsureTLSMaterial(policy, filepath.Join(dir, "pki"), spec.ServiceName, "127.0.0.1")
	if err != nil {
		return HTTPGatewayFiles{}, err
	}
	files := HTTPGatewayFiles{
		Dir: dir,
		Caddyfile: filepath.Join(dir, "Caddyfile"),
		Material: material,
	}
	config := caddyfile(spec.Upstream, spec.ContainerPort, spec.RequireClient && policy.AuthenticationRequired)
	if err := writeAtomic(files.Caddyfile, []byte(config), 0o644); err != nil {
		return HTTPGatewayFiles{}, err
	}
	return files, nil
}

func HTTPGatewayComposeService(files HTTPGatewayFiles, spec HTTPGatewaySpec) string {
	if spec.ContainerPort == 0 {
		spec.ContainerPort = 8443
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s:\n", spec.ServiceName)
	fmt.Fprintf(&b, "    image: %s\n", GatewayImage)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"65532:65532\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - /tmp:rw,noexec,nosuid,nodev\n")
	b.WriteString("      - /run/baseharbor:rw,exec,nosuid,nodev,mode=0700,uid=65532,gid=65532\n")
	b.WriteString("      - /config:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("      - /data:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("    command:\n")
	b.WriteString("      - /bin/sh\n")
	b.WriteString("      - -ec\n")
	b.WriteString("      - cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - \"127.0.0.1:$"+"{%s}:%d\"\n", spec.PublishedPortEnv, spec.ContainerPort)
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Caddyfile+":/etc/caddy/Caddyfile:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Material.ServerCertificate+":/certs/server.pem:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Material.ServerKey+":/certs/server-key.pem:ro"))
	fmt.Fprintf(&b, "      - %s\n", strconv.Quote(files.Material.CA+":/certs/ca.pem:ro"))
	if len(spec.Networks) > 0 {
		b.WriteString("    networks:\n")
		for _, network := range spec.Networks {
			if strings.TrimSpace(network) != "" {
				fmt.Fprintf(&b, "      - %s\n", network)
			}
		}
	}
	return b.String()
}

func NewHTTPClient(material TLSMaterial, requireClient bool) (*http.Client, error) {
	caPEM, err := os.ReadFile(material.CA)
	if err != nil {
		return nil, fmt.Errorf("read service access trust bundle: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("service access trust bundle contains no certificates")
	}
	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs: roots,
		ServerName: strings.TrimSpace(material.ServerName),
	}
	if requireClient {
		if material.ClientCertificate == "" || material.ClientKey == "" {
			return nil, errors.New("service access client certificate/key are required")
		}
		cert, err := tls.LoadX509KeyPair(material.ClientCertificate, material.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("load service access client identity: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}
	transport := &http.Transport{
		TLSClientConfig: config,
		TLSHandshakeTimeout: 5 * time.Second,
		DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, nil
}

func LoopbackHTTPSURL(port int) (string, error) {
	if port < 1 || port > 65535 {
		return "", errors.New("service access port is invalid")
	}
	return "https://127.0.0.1:" + strconv.Itoa(port), nil
}

func WaitHTTPS(ctx context.Context, client *http.Client, endpoint, path string) error {
	if client == nil {
		return errors.New("service access HTTP client is required")
	}
	if path == "" {
		path = "/"
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+path, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return last
		case <-ticker.C:
		}
	}
}

func caddyfile(upstream string, port int, requireClient bool) string {
	var tlsBlock string
	if requireClient {
		tlsBlock = ` {
    client_auth {
      mode require_and_verify
      trust_pool file {
        pem_file /certs/ca.pem
      }
    }
  }`
	}
	return fmt.Sprintf(`:%d {
  tls /certs/server.pem /certs/server-key.pem%s
  reverse_proxy %s
}
`, port, tlsBlock, upstream)
}
