package metrics

import (
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
	"os"
	"sort"
	"strconv"
	"strings"
)

func ProviderEndpoint(files ProviderFiles) (string, error) {
	data, err := os.ReadFile(files.Env)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "BASEHARBOR_PROMETHEUS_PORT" {
			port, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || port < 1 || port > 65535 {
				return "", errors.New("invalid Prometheus port")
			}
			return "https://127.0.0.1:" + strconv.Itoa(port), nil
		}
	}
	return "", errors.New("Prometheus port is not materialized")
}

type targetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

func targetFilePrefix(m application.Manifest) string {
	return m.Name + "--" + m.Environment + "--"
}

func targetFileName(m application.Manifest, source string) string {
	return targetFilePrefix(m) + source + ".json"
}

func providerComposeYAML(placement Placement, registrations []sourceRegistration) string {
	return providerComposeYAMLWithProviderNetworks(placement, registrations, nil, false)
}

func providerComposeYAMLWithProviderNetworks(placement Placement, registrations []sourceRegistration, providerNetworks []string, hasRuntimeCA bool) string {
	access := serviceaccess.HTTPGatewayFiles{
		Caddyfile: "./service-access/Caddyfile",
		Material: serviceaccess.TLSMaterial{
			CA:                "./service-access/runtime/ca.pem",
			ServerCertificate: "./service-access/runtime/server.pem",
			ServerKey:         "./service-access/runtime/server-key.pem",
		},
	}
	return providerComposeYAMLWithProviderNetworksAndAccess(placement, registrations, providerNetworks, hasRuntimeCA, false, access)
}

func providerComposeYAMLWithProviderNetworksAndAccess(placement Placement, registrations []sourceRegistration, providerNetworks []string, hasRuntimeCA, hasProviderSecurity bool, access serviceaccess.HTTPGatewayFiles, providerSources ...[]observability.MetricsSource) string {
	registrations = append([]sourceRegistration(nil), registrations...)
	sort.Slice(registrations, func(i, j int) bool {
		if registrations[i].Application != registrations[j].Application {
			return registrations[i].Application < registrations[j].Application
		}
		return registrations[i].Environment < registrations[j].Environment
	})

	var b strings.Builder
	b.WriteString("services:\n  prometheus:\n")
	fmt.Fprintf(&b, "    image: %s\n", ProviderImage)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"65534:65534\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    command:\n")
	b.WriteString("      - --config.file=/etc/prometheus/prometheus.yml\n")
	b.WriteString("      - --storage.tsdb.path=/prometheus\n")
	b.WriteString("      - --web.enable-lifecycle\n")
	b.WriteString("    volumes:\n")
	b.WriteString("      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro\n")
	b.WriteString("      - ./targets:/etc/prometheus/targets:ro\n")
	if hasRuntimeCA {
		b.WriteString("      - ./baseharbor-runtime-ca.pem:/etc/prometheus/baseharbor-runtime-ca.pem:ro\n")
	}
	if hasProviderSecurity {
		securityFiles := map[string]struct{}{}
		if len(providerSources) > 0 {
			for _, source := range providerSources[0] {
				if !source.Security.TLSRequired {
					continue
				}
				token := providerSourceToken(source.ID)
				securityFiles[token+"-ca.pem"] = struct{}{}
				if source.Security.ClientCertificate != "" {
					securityFiles[token+"-client.pem"] = struct{}{}
					securityFiles[token+"-client-key.pem"] = struct{}{}
				}
			}
		}
		if len(securityFiles) == 0 {
			b.WriteString("      - ./provider-security:/etc/prometheus/provider-security:ro\n")
		} else {
			names := make([]string, 0, len(securityFiles))
			for name := range securityFiles {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Fprintf(&b, "      - %s\n", strconv.Quote("./provider-security/"+name+":/etc/prometheus/provider-security/"+name+":ro"))
			}
		}
	}
	b.WriteString("      - prometheus-data:/prometheus\n")
	for i, registration := range registrations {
		if registration.RuntimeVolume != "" {
			fmt.Fprintf(&b, "      - runtime-targets-%d:/etc/prometheus/runtime-targets/%d:ro\n", i, i)
		}
	}
	b.WriteString("    tmpfs:\n      - /tmp\n")
	b.WriteString("    cap_drop:\n      - ALL\n")
	b.WriteString("    security_opt:\n      - no-new-privileges:true\n")
	b.WriteString("    networks:\n")
	b.WriteString("      - access\n")
	if len(registrations) > 0 || len(providerNetworks) > 0 {
		for i := range registrations {
			fmt.Fprintf(&b, "      - metrics-%d\n", i)
		}
		for i := range providerNetworks {
			fmt.Fprintf(&b, "      - provider-%d\n", i)
		}
	}
	b.WriteString(serviceaccess.HTTPGatewayComposeService(access, prometheusAccessSpec()))
	b.WriteString("\nnetworks:\n")
	// Keep the clear-text Prometheus backend isolated. The TLS gateway joins a
	// separate publish network so Docker/Podman can expose only its loopback
	// HTTPS port without making the backend network host-reachable.
	b.WriteString("  access:\n    internal: true\n")
	b.WriteString("  publish: {}\n")
	if len(registrations) > 0 || len(providerNetworks) > 0 {
		for i, registration := range registrations {
			fmt.Fprintf(&b, "  metrics-%d:\n    name: %s\n", i, strconv.Quote(registration.Network))
		}
		for i, network := range providerNetworks {
			fmt.Fprintf(&b, "  provider-%d:\n    external: true\n    name: %s\n", i, strconv.Quote(network))
		}
	}
	b.WriteString("\nvolumes:\n")
	fmt.Fprintf(&b, "  prometheus-data:\n    name: %s\n", strconv.Quote(placement.Volume))
	for i, registration := range registrations {
		if registration.RuntimeVolume != "" {
			fmt.Fprintf(&b, "  runtime-targets-%d:\n    external: true\n    name: %s\n", i, strconv.Quote(registration.RuntimeVolume))
		}
	}
	return b.String()
}

func prometheusConfig(registrations []sourceRegistration, hasRuntimeCA bool, providerSources ...[]observability.MetricsSource) string {
	var b strings.Builder
	b.WriteString(`global:
  scrape_interval: 5s
  scrape_timeout: 4s

scrape_configs:
  - job_name: baseharbor-applications
    file_sd_configs:
      - files:
          - /etc/prometheus/targets/*--*--*.json
`)
	for i, registration := range registrations {
		if registration.RuntimeVolume == "" {
			continue
		}
		fmt.Fprintf(&b, "          - /etc/prometheus/runtime-targets/%d/*.json\n", i)
	}
	b.WriteString(`        refresh_interval: 2s
`)
	if hasRuntimeCA {
		b.WriteString(`    tls_config:
      ca_file: /etc/prometheus/baseharbor-runtime-ca.pem
`)
	}
	b.WriteString(`    relabel_configs:
      - source_labels: [baseharbor_metrics_path]
        target_label: __metrics_path__
      - source_labels: [baseharbor_metrics_scheme]
        target_label: __scheme__
      - action: labeldrop
        regex: baseharbor_metrics_(path|scheme)

  - job_name: baseharbor-providers
    file_sd_configs:
      - files:
          - /etc/prometheus/targets/provider--*.json
        refresh_interval: 2s
    relabel_configs:
      - source_labels: [baseharbor_metrics_path]
        target_label: __metrics_path__
      - action: labeldrop
        regex: baseharbor_metrics_path
`)
	if len(providerSources) > 0 {
		for _, source := range providerSources[0] {
			if !source.Security.TLSRequired {
				continue
			}
			token := providerSourceToken(source.ID)
			fmt.Fprintf(&b, "\n  - job_name: baseharbor-provider-secure-%s\n", token)
			b.WriteString("    scheme: https\n")
			fmt.Fprintf(&b, "    metrics_path: %s\n", strconv.Quote(source.Path))
			b.WriteString("    file_sd_configs:\n")
			b.WriteString("      - files:\n")
			fmt.Fprintf(&b, "          - /etc/prometheus/targets/%s\n", providerTargetFileName(source))
			b.WriteString("        refresh_interval: 2s\n")
			b.WriteString("    tls_config:\n")
			fmt.Fprintf(&b, "      ca_file: /etc/prometheus/provider-security/%s-ca.pem\n", token)
			if source.Security.ClientCertificate != "" {
				fmt.Fprintf(&b, "      cert_file: /etc/prometheus/provider-security/%s-client.pem\n", token)
				fmt.Fprintf(&b, "      key_file: /etc/prometheus/provider-security/%s-client-key.pem\n", token)
			}
			if strings.TrimSpace(source.Security.ServerName) != "" {
				fmt.Fprintf(&b, "      server_name: %s\n", strconv.Quote(source.Security.ServerName))
			}
		}
	}
	return b.String()
}
