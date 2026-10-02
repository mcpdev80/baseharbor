package application

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	postgresHARecommendedMembers = 3
	postgresHAImage              = "ghcr.io/zalando/spilo-18:4.1-p2"
	postgresHAEtcdImage          = "gcr.io/etcd-development/etcd:v3.7.2"
)

func postgresMemberCount(m Manifest) int {
	return managedHAMemberCount(m, "sql", postgresHARecommendedMembers)
}

func postgresMemberServiceName(instance string, ordinal int) string {
	return fmt.Sprintf("%s-member-%d", runtimeServiceName("postgres", instance), ordinal+1)
}

func postgresEtcdServiceName(instance string, ordinal int) string {
	return fmt.Sprintf("%s-etcd-%d", runtimeServiceName("postgres", instance), ordinal+1)
}

func postgresAdminServiceName(instance string) string {
	return runtimeServiceName("postgres", instance) + "-admin"
}

func postgresInitServiceName(instance string) string {
	return runtimeServiceName("postgres", instance) + "-init"
}

func postgresMemberVolumeName(instance string, ordinal int) string {
	return fmt.Sprintf("%s-data", postgresMemberServiceName(instance, ordinal))
}

func postgresEtcdVolumeName(instance string, ordinal int) string {
	return fmt.Sprintf("%s-data", postgresEtcdServiceName(instance, ordinal))
}

func postgresReplicationPasswordKey(instance string) string {
	return postgresRuntimeKey(instance, "REPLICATION_PASSWORD")
}

func postgresExecutionService(m Manifest, instance string) string {
	if postgresMemberCount(m) > 1 {
		return postgresAdminServiceName(instance)
	}
	return runtimeServiceName("postgres", instance)
}

func postgresHAProxyConfig(instance string) string {
	var b strings.Builder
	b.WriteString("global\n  log stdout format raw local0\n\n")
	b.WriteString("defaults\n  mode tcp\n  log global\n  timeout connect 5s\n  timeout client 30s\n  timeout server 30s\n\n")
	b.WriteString("frontend postgres\n  bind :5432\n  default_backend primary\n\n")
	b.WriteString("backend primary\n  option httpchk GET /primary\n  http-check expect status 200\n  default-server check port 8008 inter 2s fall 2 rise 2\n")
	for ordinal := 0; ordinal < postgresHARecommendedMembers; ordinal++ {
		fmt.Fprintf(&b, "  server postgres-%d %s:5432 check\n", ordinal+1, postgresMemberServiceName(instance, ordinal))
	}
	return b.String()
}

func postgresHAProxyConfigPath(root string) string {
	return filepath.Join(root, "runtime", "haproxy.cfg")
}
