package application

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

const valkeySentinelMasterName = "baseharbor"

func valkeyAvailabilityComponent(m Manifest, instance string) string {
	for _, name := range KeyValueInstanceNames(m) {
		if name == instance {
			return "key_value"
		}
	}
	return "cache"
}

func valkeyMemberCount(m Manifest, instance string) int {
	return managedHAMemberCount(m, valkeyAvailabilityComponent(m, instance), 3)
}

func valkeyMemberServiceName(instance string, ordinal int) string {
	base := runtimeServiceName("valkey", instance)
	if ordinal <= 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, ordinal+1)
}

func valkeyMemberVolumeName(instance string, ordinal int) string {
	return valkeyMemberServiceName(instance, ordinal) + "-data"
}

func valkeySentinelServiceName(instance string, ordinal int) string {
	base := runtimeServiceName("valkey", instance) + "-sentinel"
	if ordinal <= 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, ordinal+1)
}

func valkeyGatewayUpstreams(m Manifest, instance string) []serviceaccess.TCPGatewayUpstream {
	count := valkeyMemberCount(m, instance)
	out := make([]serviceaccess.TCPGatewayUpstream, 0, count)
	for ordinal := 0; ordinal < count; ordinal++ {
		service := valkeyMemberServiceName(instance, ordinal)
		out = append(out, serviceaccess.TCPGatewayUpstream{Name: service, Host: service, Port: 6379})
	}
	return out
}

func valkeyGatewaySpec(m Manifest, instance string) serviceaccess.TCPGatewaySpec {
	spec := serviceaccess.TCPGatewaySpec{
		ServiceName:      valkeyAccessService(instance),
		UpstreamHost:     runtimeServiceName("valkey", instance),
		UpstreamPort:     6379,
		Upstreams:        valkeyGatewayUpstreams(m, instance),
		PublishedPortEnv: valkeyRuntimeKey(instance, "HOST_PORT"),
		ContainerPort:    6379,
	}
	if valkeyMemberCount(m, instance) > 1 {
		spec.Environment = map[string]string{
			"VALKEY_HEALTH_PASSWORD": "${" + valkeyRuntimeKey(instance, "PASSWORD") + "}",
		}
		spec.BackendDirectives = []string{
			"option tcp-check",
			"tcp-check connect",
			"tcp-check send-lf \"AUTH %[env(VALKEY_HEALTH_PASSWORD)]\\r\\n\"",
			"tcp-check expect string +OK",
			"tcp-check send-lf \"INFO replication\\r\\n\"",
			"tcp-check expect string role:master",
			"tcp-check send-lf \"QUIT\\r\\n\"",
			"tcp-check expect string +OK",
		}
		spec.ServerDirectives = []string{"init-state fully-down", "inter 1s", "rise 1", "fall 1"}
	}
	return spec
}

func writeValkeyHAComposeServices(b *strings.Builder, m Manifest, instance string) {
	passwordKey := valkeyRuntimeKey(instance, "PASSWORD")
	count := valkeyMemberCount(m, instance)
	primary := valkeyMemberServiceName(instance, 0)
	for ordinal := 0; ordinal < count; ordinal++ {
		service := valkeyMemberServiceName(instance, ordinal)
		fmt.Fprintf(b, "  %s:\n", service)
		b.WriteString("    image: docker.io/valkey/valkey:9.1.2-alpine\n")
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    user: \"999:1000\"\n")
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n")
		b.WriteString("    environment:\n")
		fmt.Fprintf(b, "      VALKEY_PASSWORD: ${%s}\n", passwordKey)
		b.WriteString("    command:\n      - sh\n      - -ec\n      - |\n")
		b.WriteString("        {\n")
		b.WriteString("          printf 'requirepass %s\\n' \"$VALKEY_PASSWORD\"\n")
		b.WriteString("          printf 'masterauth %s\\n' \"$VALKEY_PASSWORD\"\n")
		b.WriteString("          printf 'appendonly yes\\n'\n")
		b.WriteString("          printf 'dir /data\\n'\n")
		if ordinal > 0 {
			fmt.Fprintf(b, "          printf 'replicaof %s 6379\\n'\n", primary)
		}
		b.WriteString("        } > /tmp/valkey.conf\n")
		b.WriteString("        exec valkey-server /tmp/valkey.conf\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(b, "      - %s:/data\n", valkeyMemberVolumeName(instance, ordinal))
		fmt.Fprintf(b, "      - ./bindings/valkey/%s/ca.pem:/run/baseharbor/tls/ca.pem:ro\n", instance)
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD-SHELL\", \"valkey-cli ping 2>&1 | grep -Eq '^PONG$|^NOAUTH '\" ]\n")
		b.WriteString("      interval: 5s\n      timeout: 5s\n      retries: 12\n      start_period: 5s\n\n")
	}
	if count <= 1 {
		return
	}
	for ordinal := 0; ordinal < 3; ordinal++ {
		service := valkeySentinelServiceName(instance, ordinal)
		fmt.Fprintf(b, "  %s:\n", service)
		b.WriteString("    image: docker.io/valkey/valkey:9.1.2-alpine\n")
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    user: \"999:1000\"\n")
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n")
		b.WriteString("    environment:\n")
		fmt.Fprintf(b, "      VALKEY_PASSWORD: ${%s}\n", passwordKey)
		b.WriteString("    command:\n      - sh\n      - -ec\n      - |\n")
		b.WriteString("        primary_ip=\"\"\n")
		b.WriteString("        attempt=0\n")
		b.WriteString("        until [ -n \"$$primary_ip\" ]; do\n")
		fmt.Fprintf(b, "          primary_addr=\"$(VALKEYCLI_AUTH=\"$$VALKEY_PASSWORD\" valkey-cli -h %s -p 6379 --raw CLIENT INFO 2>/dev/null || true)\"\n", primary)
		b.WriteString("          primary_ip=\"$(printf '%s\\n' \"$$primary_addr\" | tr ' ' '\\n' | sed -n 's/^laddr=\\([^:]*\\):.*/\\1/p' | head -n1)\"\n")
		b.WriteString("          if [ -z \"$$primary_ip\" ]; then attempt=$$((attempt+1)); [ \"$$attempt\" -lt 60 ] || exit 1; sleep 1; fi\n")
		b.WriteString("        done\n")
		b.WriteString("        {\n")
		b.WriteString("          printf 'port 26379\\n'\n")
		b.WriteString("          printf 'protected-mode no\\n'\n")
		fmt.Fprintf(b, "          printf 'sentinel monitor %s %%s 6379 2\\n' \"$$primary_ip\"\n", valkeySentinelMasterName)
		fmt.Fprintf(b, "          printf 'sentinel auth-pass %s %%s\\n' \"$$VALKEY_PASSWORD\"\n", valkeySentinelMasterName)
		fmt.Fprintf(b, "          printf 'sentinel down-after-milliseconds %s 5000\\n'\n", valkeySentinelMasterName)
		fmt.Fprintf(b, "          printf 'sentinel failover-timeout %s 15000\\n'\n", valkeySentinelMasterName)
		fmt.Fprintf(b, "          printf 'sentinel parallel-syncs %s 1\\n'\n", valkeySentinelMasterName)
		b.WriteString("        } > /tmp/sentinel.conf\n")
		b.WriteString("        exec valkey-sentinel /tmp/sentinel.conf\n")
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD-SHELL\", \"valkey-cli -p 26379 ping | grep -q '^PONG$'\"]\n")
		b.WriteString("      interval: 5s\n      timeout: 5s\n      retries: 12\n      start_period: 5s\n\n")
	}
}

func sharedValkeyMemberCount(resource sharedValkeyResource) int {
	if resource.Instances > 1 {
		return resource.Instances
	}
	return 1
}

func sharedValkeyMemberServiceName(app sharedBackendAppState, instance string, ordinal int) string {
	base := sharedValkeyServiceFor(app.Application, app.Environment, instance)
	if ordinal <= 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, ordinal+1)
}

func sharedValkeyMemberVolumeName(app sharedBackendAppState, instance string, ordinal int) string {
	return sharedValkeyMemberServiceName(app, instance, ordinal) + "-data"
}

func sharedValkeySentinelServiceName(app sharedBackendAppState, instance string, ordinal int) string {
	base := sharedValkeyServiceFor(app.Application, app.Environment, instance) + "-sentinel"
	if ordinal <= 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, ordinal+1)
}

func sharedValkeyGatewaySpec(app sharedBackendAppState, instance string) serviceaccess.TCPGatewaySpec {
	resource := app.Cache[instance]
	service := sharedValkeyServiceFor(app.Application, app.Environment, instance)
	spec := serviceaccess.TCPGatewaySpec{
		ServiceName:      sharedValkeyAccessServiceFor(app.Application, app.Environment, instance),
		UpstreamHost:     service,
		UpstreamPort:     6379,
		PublishedPortEnv: sharedValkeyPortEnvFor(app.Application, app.Environment, instance),
		ContainerPort:    6379,
		Network:          "shared-backend",
	}
	count := sharedValkeyMemberCount(resource)
	if count <= 1 {
		return spec
	}
	spec.Upstreams = make([]serviceaccess.TCPGatewayUpstream, 0, count)
	for ordinal := 0; ordinal < count; ordinal++ {
		member := sharedValkeyMemberServiceName(app, instance, ordinal)
		spec.Upstreams = append(spec.Upstreams, serviceaccess.TCPGatewayUpstream{Name: member, Host: member, Port: 6379})
	}
	spec.Environment = map[string]string{
		"VALKEY_HEALTH_PASSWORD": "${" + sharedValkeyPasswordEnvFor(app.Application, app.Environment, instance) + "}",
	}
	spec.BackendDirectives = []string{
		"option tcp-check",
		"tcp-check connect",
		"tcp-check send-lf \"AUTH %[env(VALKEY_HEALTH_PASSWORD)]\\r\\n\"",
		"tcp-check expect string +OK",
		"tcp-check send-lf \"INFO replication\\r\\n\"",
		"tcp-check expect string role:master",
		"tcp-check send-lf \"QUIT\\r\\n\"",
		"tcp-check expect string +OK",
	}
	spec.ServerDirectives = []string{"init-state fully-down", "inter 1s", "rise 1", "fall 1"}
	return spec
}
