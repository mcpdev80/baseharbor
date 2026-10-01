package devgateway

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func renderCaddyfile(routes []Route, listenPort int) string {
	var b strings.Builder
	b.WriteString("{\n  auto_https off\n}\n")
	renderListener := func(port int) {
		fmt.Fprintf(&b, "\n:%d {\n  tls /certs/server.pem /certs/server-key.pem\n", port)
		for i, route := range routes {
			fmt.Fprintf(&b, "  @route%d {\n    host %s\n", i, route.Host)
			if route.PathPrefix != "" {
				fmt.Fprintf(&b, "    path %s %s/*\n", route.PathPrefix, route.PathPrefix)
			}
			b.WriteString("  }\n")
			fmt.Fprintf(&b, "  handle @route%d {\n", i)
			if route.PathPrefix != "" {
				fmt.Fprintf(&b, "    uri strip_prefix %s\n", route.PathPrefix)
			}
			if strings.HasPrefix(route.Upstream, "https://") {
				fmt.Fprintf(&b, "    reverse_proxy %s {\n", route.Upstream)
				b.WriteString("      transport http {\n        tls\n")
				fmt.Fprintf(&b, "        tls_trust_pool file /trust/route-%03d.pem\n", i)
				fmt.Fprintf(&b, "        tls_server_name %s\n", route.ServerName)
				b.WriteString("      }\n    }\n")
			} else {
				fmt.Fprintf(&b, "    reverse_proxy %s\n", route.Upstream)
			}
			b.WriteString("  }\n")
		}
		b.WriteString("  respond 404\n}\n")
	}
	renderListener(listenPort)
	return b.String()
}

func renderCompose(files Files, routes []Route, trustTargets map[string]string, hostPort int) string {
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
	b.WriteString("    cap_drop: [\"ALL\"]\n    cap_add: [\"NET_BIND_SERVICE\"]\n    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n      - /run/baseharbor:rw,exec,nosuid,nodev,mode=1777\n      - /config:rw,noexec,nosuid,nodev,mode=1777\n      - /data:rw,noexec,nosuid,nodev,mode=1777\n")
	b.WriteString("    entrypoint: [\"/bin/sh\", \"-ec\"]\n")
	b.WriteString("    command:\n      - cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile\n")
	fmt.Fprintf(&b, "    ports:\n      - \"127.0.0.1:%d:%d\"\n", hostPort, hostPort)
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %q\n", files.Caddyfile+":/etc/caddy/Caddyfile:ro")
	fmt.Fprintf(&b, "      - %q\n", files.Cert+":/certs/server.pem:ro")
	fmt.Fprintf(&b, "      - %q\n", files.Key+":/certs/server-key.pem:ro")
	for _, route := range routes {
		if trust := trustTargets[route.Key]; trust != "" {
			fmt.Fprintf(&b, "      - %q\n", trust+":/trust/"+trustMountName(routes, route.Key)+":ro")
		}
	}
	b.WriteString("    networks:\n")
	seen := map[string]bool{}
	for _, route := range routes {
		logical := routeNetwork[route.Key]
		if seen[logical] {
			continue
		}
		seen[logical] = true
		fmt.Fprintf(&b, "      %s:\n", logical)
		b.WriteString("        aliases:\n")
		for _, candidate := range routes {
			if routeNetwork[candidate.Key] == logical {
				fmt.Fprintf(&b, "          - %q\n", candidate.Host)
			}
		}
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
