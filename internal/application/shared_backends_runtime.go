package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func waitSharedValkeyReady(ctx context.Context, compose bhruntime.RuntimeProvider, shared SharedBackendFiles, m Manifest) error {
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for _, instance := range ValkeyInstanceNames(m) {
		app := sharedBackendApplicationKey(m)
		state, err := loadSharedBackendState(shared.State, m.Environment)
		if err != nil {
			return err
		}
		resource := state.Applications[app].Cache[instance]
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return fmt.Errorf("load shared Valkey credential %s: %w", instance, err)
		}
		service := sharedValkeyService(m, instance)
		ticker := time.NewTicker(500 * time.Millisecond)
		var lastErr error
		ready := false
		for !ready {
			script := fmt.Sprintf("VALKEYCLI_AUTH=%s valkey-cli -h 127.0.0.1 -p 6379 ping", shellQuote(password))
			out, execErr := compose.ExecProject(waitCtx, shared.Project, shared.Compose, shared.Env, service, "sh", "-ec", script)
			if execErr == nil && strings.TrimSpace(out) == "PONG" {
				ready = true
				break
			}
			if execErr != nil {
				lastErr = execErr
			} else {
				lastErr = fmt.Errorf("unexpected readiness result %q", strings.TrimSpace(out))
			}
			select {
			case <-waitCtx.Done():
				ticker.Stop()
				if lastErr != nil {
					return fmt.Errorf("wait for shared Valkey %s readiness: %w", instance, lastErr)
				}
				return fmt.Errorf("wait for shared Valkey %s readiness: %w", instance, waitCtx.Err())
			case <-ticker.C:
			}
		}
		ticker.Stop()
	}
	return nil
}

func waitSharedPostgresReady(ctx context.Context, compose bhruntime.RuntimeProvider, shared SharedBackendFiles, environment string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		out, err := compose.ExecProject(
			waitCtx,
			shared.Project,
			shared.Compose,
			shared.Env,
			sharedPostgresService(environment),
			"pg_isready",
			"-h", sharedPostgresAlias(),
			"-p", "5432",
			"-U", "postgres",
			"-d", "postgres",
		)
		if err == nil && strings.Contains(strings.ToLower(strings.TrimSpace(out)), "accepting connections") {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("unexpected readiness result %q", strings.TrimSpace(out))
		}

		select {
		case <-waitCtx.Done():
			cause := lastErr
			if cause == nil {
				cause = waitCtx.Err()
			}
			diagnosticCtx, diagnosticCancel := context.WithTimeout(context.Background(), 5*time.Second)
			details := strings.TrimSpace(compose.DiagnosticsProject(diagnosticCtx, shared.Project, shared.Compose, shared.Env))
			diagnosticCancel()
			if details != "" {
				return fmt.Errorf("wait for shared PostgreSQL readiness: %w; runtime diagnostics:\n%s", cause, details)
			}
			return fmt.Errorf("wait for shared PostgreSQL readiness: %w", cause)
		case <-ticker.C:
		}
	}
}

func ensureSharedPostgresAdminIdentity(ctx context.Context, compose bhruntime.RuntimeProvider, shared SharedBackendFiles, environment string) error {
	script := `set -eu
export PGPASSWORD="$SHARED_POSTGRES_SUPERUSER_PASSWORD"
sql="SELECT format('CREATE ROLE %I LOGIN SUPERUSER PASSWORD %L', 'baseharbor_admin', '$SHARED_POSTGRES_ADMIN_PASSWORD') WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'baseharbor_admin')\\gexec
ALTER ROLE baseharbor_admin WITH LOGIN SUPERUSER PASSWORD '$SHARED_POSTGRES_ADMIN_PASSWORD';"
printf '%s\n' "$sql" | psql -h postgres-access -U postgres -d postgres -v ON_ERROR_STOP=1
`
	if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(environment), "sh", "-ec", script); err != nil {
		return fmt.Errorf("ensure shared PostgreSQL provider administrator: %w", err)
	}
	return nil
}

func reconcileSharedPostgresApplication(ctx context.Context, compose bhruntime.RuntimeProvider, shared SharedBackendFiles, app sharedBackendAppState) error {
	instances := make([]string, 0, len(app.SQL))
	for instance := range app.SQL {
		instances = append(instances, instance)
	}
	sort.Strings(instances)
	for _, instance := range instances {
		resource := app.SQL[instance]
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return fmt.Errorf("load shared PostgreSQL credential %s: %w", instance, err)
		}
		sql := fmt.Sprintf("REVOKE ALL ON DATABASE postgres FROM PUBLIC;\nREVOKE ALL ON DATABASE template1 FROM PUBLIC;\nSELECT format('CREATE ROLE %%I LOGIN PASSWORD %%L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS', %s, %s) WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = %s)\\gexec\nALTER ROLE %s WITH LOGIN PASSWORD %s NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION NOBYPASSRLS;\nSELECT format('CREATE DATABASE %%I OWNER %%I', %s, %s) WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = %s)\\gexec\nALTER DATABASE %s OWNER TO %s;\nREVOKE ALL ON DATABASE %s FROM PUBLIC;\nGRANT CONNECT, TEMPORARY ON DATABASE %s TO %s;\n",
			quotePostgresLiteral(resource.Username), quotePostgresLiteral(password), quotePostgresLiteral(resource.Username),
			quotePostgresIdent(resource.Username), quotePostgresLiteral(password),
			quotePostgresLiteral(resource.Database), quotePostgresLiteral(resource.Username), quotePostgresLiteral(resource.Database),
			quotePostgresIdent(resource.Database), quotePostgresIdent(resource.Username),
			quotePostgresIdent(resource.Database),
			quotePostgresIdent(resource.Database), quotePostgresIdent(resource.Username),
		)
		if _, err := compose.ExecProjectInput(ctx, shared.Project, shared.Compose, shared.Env, []byte(sql), sharedPostgresService(app.Environment), "psql", "-h", sharedPostgresAlias(), "-U", "baseharbor_admin", "-d", "postgres", "-v", "ON_ERROR_STOP=1"); err != nil {
			return fmt.Errorf("reconcile shared PostgreSQL resource %s: %w", instance, err)
		}
		harden := fmt.Sprintf("REVOKE ALL ON SCHEMA public FROM PUBLIC; GRANT USAGE, CREATE ON SCHEMA public TO %s; REVOKE ALL ON ALL TABLES IN SCHEMA public FROM PUBLIC; REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC; REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON TABLES FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON SEQUENCES FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON FUNCTIONS FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON TYPES FROM PUBLIC;",
			quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username))
		if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(app.Environment), "psql", "-h", sharedPostgresAlias(), "-U", "baseharbor_admin", "-d", resource.Database, "-v", "ON_ERROR_STOP=1", "-c", harden); err != nil {
			return fmt.Errorf("harden shared PostgreSQL resource %s: %w", instance, err)
		}
	}
	return nil
}

func renderSharedBackendRuntime(files SharedBackendFiles, state sharedBackendState) error {
	var env strings.Builder
	if state.PostgresAdminCredential != "" {
		password, err := readSharedBackendCredential(files.Dir, state.PostgresAdminCredential)
		if err != nil {
			return err
		}
		fmt.Fprintf(&env, "SHARED_POSTGRES_ADMIN_PASSWORD=%s\n", password)
	}
	if state.PostgresSuperuserCredential != "" {
		password, err := readSharedBackendCredential(files.Dir, state.PostgresSuperuserCredential)
		if err != nil {
			return err
		}
		fmt.Fprintf(&env, "SHARED_POSTGRES_SUPERUSER_PASSWORD=%s\n", password)
	}
	if state.PostgresReplicationCredential != "" {
		password, err := readSharedBackendCredential(files.Dir, state.PostgresReplicationCredential)
		if err != nil {
			return err
		}
		fmt.Fprintf(&env, "SHARED_POSTGRES_REPLICATION_PASSWORD=%s\n", password)
	}
	if state.PostgresHostPort > 0 {
		fmt.Fprintf(&env, "SHARED_POSTGRES_HOST_PORT=%d\n", state.PostgresHostPort)
	}
	if state.PostgresUIHostPort > 0 {
		fmt.Fprintf(&env, "SHARED_POSTGRES_UI_HOST_PORT=%d\n", state.PostgresUIHostPort)
	}
	if state.CacheUIHostPort > 0 {
		fmt.Fprintf(&env, "SHARED_CACHE_UI_HOST_PORT=%d\n", state.CacheUIHostPort)
	}
	if state.ManagementUsername != "" {
		fmt.Fprintf(&env, "SHARED_MANAGEMENT_USER=%s\n", state.ManagementUsername)
		fmt.Fprintf(&env, "SHARED_PGADMIN_EMAIL=%s\n", developmentPostgresUIEmail(state.ManagementUsername))
	}
	if state.ManagementPassword != "" {
		fmt.Fprintf(&env, "SHARED_MANAGEMENT_PASSWORD=%s\n", state.ManagementPassword)
	}
	appKeys := make([]string, 0, len(state.Applications))
	for key := range state.Applications {
		appKeys = append(appKeys, key)
	}
	sort.Strings(appKeys)
	for _, key := range appKeys {
		app := state.Applications[key]
		for instance, resource := range app.Cache {
			password, err := readSharedBackendCredential(files.Dir, resource.CredentialReference)
			if err != nil {
				return err
			}
			fmt.Fprintf(&env, "%s=%d\n", sharedValkeyPortEnvFor(app.Application, app.Environment, instance), resource.HostPort)
			fmt.Fprintf(&env, "%s=%s\n", sharedValkeyPasswordEnvFor(app.Application, app.Environment, instance), password)
		}
	}
	if err := writeOwnerOnlyFile(files.Env, []byte(env.String())); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("services:\n")
	hasPostgres := state.PostgresAdminCredential != ""
	if !hasPostgres {
		for _, app := range state.Applications {
			if len(app.SQL) > 0 {
				hasPostgres = true
				break
			}
		}
	}
	if hasPostgres {
		writeSharedPostgresCompose(&b, state)
	}
	if sharedBackendPostgresUIRequested(state) {
		writeSharedPostgresUICompose(&b)
	}
	if sharedBackendCacheUIRequested(state) {
		writeSharedCacheUICompose(&b)
	}
	for _, key := range appKeys {
		app := state.Applications[key]
		instances := make([]string, 0, len(app.Cache))
		for instance := range app.Cache {
			instances = append(instances, instance)
		}
		sort.Strings(instances)
		for _, instance := range instances {
			writeSharedValkeyCompose(&b, app, instance)
		}
	}
	b.WriteString("volumes:\n")
	if hasPostgres {
		for ordinal := 1; ordinal <= 3; ordinal++ {
			fmt.Fprintf(&b, "  shared-postgres-data-%d:\n    name: %s-%s-postgres-data-%d\n", ordinal, files.ResourceProject, sharedBackendToken(state.Environment), ordinal)
			fmt.Fprintf(&b, "  shared-postgres-etcd-data-%d:\n    name: %s-%s-postgres-etcd-data-%d\n", ordinal, files.ResourceProject, sharedBackendToken(state.Environment), ordinal)
		}
	}
	for _, key := range appKeys {
		app := state.Applications[key]
		for instance, resource := range app.Cache {
			for ordinal := 0; ordinal < sharedValkeyMemberCount(resource); ordinal++ {
				volume := sharedValkeyMemberVolumeName(app, instance, ordinal)
				fmt.Fprintf(&b, "  %s:\n    name: %s-%s-%s\n", volume, files.ResourceProject, sharedBackendToken(state.Environment), volume)
			}
		}
	}
	b.WriteString("networks:\n  shared-backend:\n")
	fmt.Fprintf(&b, "    name: %s\n", files.Network)
	return os.WriteFile(files.Compose, []byte(b.String()), 0o600)
}

func writeSharedPostgresUICompose(b *strings.Builder) {
	b.WriteString(`  shared-pgadmin:
    image: ` + PostgresUIImage + `
    restart: unless-stopped
    user: "5050:5050"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    environment:
      PGADMIN_DEFAULT_EMAIL: ${SHARED_PGADMIN_EMAIL}
      PGADMIN_DEFAULT_PASSWORD_FILE: /run/baseharbor/password
      PGADMIN_ENABLE_TLS: "True"
      PGADMIN_LISTEN_PORT: "8443"
      PGADMIN_SERVER_JSON_FILE: /run/baseharbor/servers.json
      PGADMIN_REPLACE_SERVERS_ON_STARTUP: "True"
      PGADMIN_DISABLE_POSTFIX: "True"
      PGADMIN_CUSTOM_CONFIG_DISTRO_FILE: /var/lib/pgadmin/config_distro.py
      PGADMIN_CONFIG_MASTER_PASSWORD_REQUIRED: "False"
      PGPASS_FILE: /run/baseharbor/pgpass
    ports:
      - "127.0.0.1:${SHARED_POSTGRES_UI_HOST_PORT}:8443"
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /var/lib/pgadmin:rw,noexec,nosuid,nodev,mode=1777
    volumes:
      - ./management-ui/postgres/password:/run/baseharbor/password:ro
      - ./management-ui/postgres/servers.json:/run/baseharbor/servers.json:ro
      - ./management-ui/postgres/pgpass:/run/baseharbor/pgpass:ro
      - ./management-ui/postgres/postgres.ca.pem:/run/baseharbor/postgres.ca.pem:ro
      - ./management-ui/postgres/server.cert:/certs/server.cert:ro
      - ./management-ui/postgres/server.key:/certs/server.key:ro
    networks:
      shared-backend:
        aliases:
          - shared-pgadmin

`)
}

func writeSharedCacheUICompose(b *strings.Builder) {
	b.WriteString(`  shared-cache-ui:
    image: ` + CacheUIImage + `
    restart: unless-stopped
    user: "redis"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    environment:
      HTTP_USER: ${SHARED_MANAGEMENT_USER}
      HTTP_PASSWORD_FILE: /run/baseharbor/http-password
      NOSAVE: "true"
      NO_LOG_DATA: "true"
      NODE_ENV: production
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    volumes:
      - ./management-ui/cache/http-password:/run/baseharbor/http-password:ro
      - ./management-ui/cache/local.json:/redis-commander/config/local.json:ro
      - ./management-ui/cache/local-production.json:/redis-commander/config/local-production.json:ro
    networks:
      shared-backend: {}

  shared-cache-ui-access:
    image: ` + UIProxyImage + `
    restart: unless-stopped
    user: "65532:65532"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    entrypoint: ["/bin/sh", "-ec"]
    command:
      - cat /usr/bin/caddy > /run/baseharbor/caddy && chmod 0755 /run/baseharbor/caddy && exec /run/baseharbor/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /run/baseharbor:rw,exec,nosuid,nodev,mode=1777
      - /data:rw,noexec,nosuid,nodev,mode=1777
      - /config:rw,noexec,nosuid,nodev,mode=1777
    ports:
      - "127.0.0.1:${SHARED_CACHE_UI_HOST_PORT}:8443"
    volumes:
      - ./management-ui/cache/Caddyfile:/etc/caddy/Caddyfile:ro
      - ./management-ui/cache/server.pem:/certs/server.pem:ro
      - ./management-ui/cache/server-key.pem:/certs/server-key.pem:ro
    networks:
      shared-backend:
        aliases:
          - shared-cache-ui-access

`)
}

func writeSharedPostgresCompose(b *strings.Builder, state sharedBackendState) {
	service := sharedPostgresService(state.Environment)
	cluster := "baseharbor-" + sharedBackendToken(state.Environment) + "-postgres"
	etcdCluster := make([]string, 0, 3)
	for ordinal := 1; ordinal <= 3; ordinal++ {
		name := sharedPostgresEtcdService(state.Environment, ordinal)
		etcdCluster = append(etcdCluster, fmt.Sprintf("%s=http://%s:2380", name, name))
	}
	etcdInitialCluster := strings.Join(etcdCluster, ",")
	etcdHosts := make([]string, 0, 3)
	for ordinal := 1; ordinal <= 3; ordinal++ {
		etcdHosts = append(etcdHosts, sharedPostgresEtcdService(state.Environment, ordinal)+":2379")
	}

	for ordinal := 1; ordinal <= 3; ordinal++ {
		name := sharedPostgresEtcdService(state.Environment, ordinal)
		fmt.Fprintf(b, "  %s:\n", name)
		b.WriteString("    image: gcr.io/etcd-development/etcd:v3.7.2\n")
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    command:\n")
		fmt.Fprintf(b, "      - /usr/local/bin/etcd\n      - --name=%s\n", name)
		b.WriteString("      - --data-dir=/etcd-data\n")
		b.WriteString("      - --listen-client-urls=http://0.0.0.0:2379\n")
		fmt.Fprintf(b, "      - --advertise-client-urls=http://%s:2379\n", name)
		b.WriteString("      - --listen-peer-urls=http://0.0.0.0:2380\n")
		fmt.Fprintf(b, "      - --initial-advertise-peer-urls=http://%s:2380\n", name)
		fmt.Fprintf(b, "      - --initial-cluster=%s\n", etcdInitialCluster)
		fmt.Fprintf(b, "      - --initial-cluster-token=%s\n", cluster)
		b.WriteString("      - --initial-cluster-state=new\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(b, "      - shared-postgres-etcd-data-%d:/etcd-data\n", ordinal)
		b.WriteString("    networks:\n      shared-backend: {}\n\n")
	}

	for ordinal := 1; ordinal <= 3; ordinal++ {
		name := sharedPostgresMemberService(state.Environment, ordinal)
		fmt.Fprintf(b, "  %s:\n", name)
		b.WriteString("    image: ghcr.io/zalando/spilo-18:4.1-p2\n")
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    read_only: false\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    entrypoint: [\"/bin/sh\", \"-ec\"]\n")
		b.WriteString("    command:\n")
		b.WriteString("      - |\n")
		b.WriteString("        uid=$$(id -u postgres); gid=$$(id -g postgres)\n")
		b.WriteString("        chmod 0755 /run/baseharbor\n")
		b.WriteString("        install -d -o \"$$uid\" -g \"$$gid\" -m 0750 /run/baseharbor/tls\n")
		b.WriteString("        cp /run/baseharbor/tls-source/server-cert.pem /run/baseharbor/tls/server-cert.pem\n")
		b.WriteString("        cp /run/baseharbor/tls-source/server-key.pem /run/baseharbor/tls/server-key.pem\n")
		b.WriteString("        chown \"0:$$gid\" /run/baseharbor/tls/server-cert.pem /run/baseharbor/tls/server-key.pem\n")
		b.WriteString("        chmod 0644 /run/baseharbor/tls/server-cert.pem\n")
		b.WriteString("        chmod 0640 /run/baseharbor/tls/server-key.pem\n")
		b.WriteString("        stat -c 'baseharbor=%U:%G %a' /run/baseharbor\n")
		b.WriteString("        stat -c 'tls=%U:%G %a' /run/baseharbor/tls\n")
		b.WriteString("        stat -c 'key=%U:%G %a' /run/baseharbor/tls/server-key.pem\n")
		b.WriteString("        exec /bin/sh /launch.sh init\n")
		b.WriteString("    tmpfs:\n")
		b.WriteString("      - /run/baseharbor/tls:rw,noexec,nosuid,nodev,mode=0750\n")
		b.WriteString("    environment:\n")
		b.WriteString("      SPILO_PROVIDER: local\n")
		fmt.Fprintf(b, "      SCOPE: %s\n", strconv.Quote(cluster))
		b.WriteString("      PGVERSION: \"18\"\n")
		fmt.Fprintf(b, "      ETCD3_HOSTS: %s\n", strconv.Quote(strings.Join(etcdHosts, ",")))
		b.WriteString("      PGUSER_SUPERUSER: postgres\n")
		b.WriteString("      PGPASSWORD_SUPERUSER: ${SHARED_POSTGRES_SUPERUSER_PASSWORD}\n")
		b.WriteString("      PGUSER_STANDBY: baseharbor_replication\n")
		b.WriteString("      PGPASSWORD_STANDBY: ${SHARED_POSTGRES_REPLICATION_PASSWORD}\n")
		b.WriteString("      USE_ADMIN: \"false\"\n")
		b.WriteString("      ALLOW_NOSSL: \"true\"\n")
		fmt.Fprintf(b, "      SPILO_CONFIGURATION: %s\n", strconv.Quote(fmt.Sprintf(`{"postgresql":{"connect_address":"%s:5432"},"restapi":{"connect_address":"%s:8008"}}`, name, name)))
		b.WriteString("      SSL_CERTIFICATE_FILE: /run/baseharbor/tls/server-cert.pem\n")
		b.WriteString("      SSL_PRIVATE_KEY_FILE: /run/baseharbor/tls/server-key.pem\n")
		b.WriteString("      SSL_TEST_RELOAD: \"true\"\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(b, "      - shared-postgres-data-%d:/home/postgres/pgroot\n", ordinal)
		b.WriteString("      - ./postgresql/runtime:/run/baseharbor/tls-source:ro\n")
		b.WriteString("    networks:\n      shared-backend: {}\n")
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: [\"CMD-SHELL\", \"pg_isready -h 127.0.0.1 -p 5432 -U postgres -d postgres\"]\n")
		b.WriteString("      interval: 5s\n      timeout: 5s\n      retries: 24\n      start_period: 10s\n\n")
	}

	fmt.Fprintf(b, "  %s:\n", sharedPostgresAlias())
	fmt.Fprintf(b, "    image: %s\n", serviceaccess.TCPGatewayImage)
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    user: \"99:99\"\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs: [\"/tmp:rw,noexec,nosuid,nodev\"]\n")
	b.WriteString("    command: [\"haproxy\", \"-W\", \"-db\", \"-f\", \"/usr/local/etc/haproxy/haproxy.cfg\"]\n")
	b.WriteString("    ports:\n      - \"127.0.0.1:${SHARED_POSTGRES_HOST_PORT}:5432\"\n")
	b.WriteString("    volumes:\n      - ./postgresql/service-access/haproxy.cfg:/usr/local/etc/haproxy/haproxy.cfg:ro\n")
	b.WriteString("    networks:\n      shared-backend:\n        aliases:\n          - postgres-access\n\n")

	// Stable admin toolbox: all BaseHarbor reconciliation commands execute here
	// and connect through the same primary-aware endpoint used by applications.
	fmt.Fprintf(b, "  %s:\n", service)
	b.WriteString("    image: docker.io/library/postgres:18-alpine\n")
	b.WriteString("    restart: unless-stopped\n")
	b.WriteString("    command: [\"sh\", \"-ec\", \"trap : TERM INT; sleep infinity & wait\"]\n")
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n      - /var/run/postgresql:rw,noexec,nosuid,nodev\n")
	b.WriteString("    environment:\n      SHARED_POSTGRES_ADMIN_PASSWORD: ${SHARED_POSTGRES_ADMIN_PASSWORD}\n      SHARED_POSTGRES_SUPERUSER_PASSWORD: ${SHARED_POSTGRES_SUPERUSER_PASSWORD}\n      PGPASSWORD: ${SHARED_POSTGRES_ADMIN_PASSWORD}\n")
	b.WriteString("    networks:\n      shared-backend: {}\n")
}

func writeSharedValkeyCompose(b *strings.Builder, app sharedBackendAppState, instance string) {
	resource := app.Cache[instance]
	passwordEnv := sharedValkeyPasswordEnvFor(app.Application, app.Environment, instance)
	root := "./" + filepath.ToSlash(filepath.Join("valkey", sharedBackendToken(app.Application), sharedBackendToken(instance)))
	count := sharedValkeyMemberCount(resource)
	primary := sharedValkeyMemberServiceName(app, instance, 0)
	for ordinal := 0; ordinal < count; ordinal++ {
		service := sharedValkeyMemberServiceName(app, instance, ordinal)
		fmt.Fprintf(b, "  %s:\n", service)
		b.WriteString("    image: docker.io/valkey/valkey:9.1.2-alpine\n")
		b.WriteString("    restart: unless-stopped\n")
		b.WriteString("    user: \"999:1000\"\n")
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n")
		b.WriteString("    environment:\n")
		fmt.Fprintf(b, "      VALKEY_PASSWORD: ${%s}\n", passwordEnv)
		b.WriteString("    command:\n      - sh\n      - -ec\n      - |\n")
		b.WriteString("        {\n")
		b.WriteString("          printf 'requirepass %s\\n' \"$$VALKEY_PASSWORD\"\n")
		b.WriteString("          printf 'masterauth %s\\n' \"$$VALKEY_PASSWORD\"\n")
		b.WriteString("          printf 'appendonly yes\\n'\n")
		b.WriteString("          printf 'dir /data\\n'\n")
		if ordinal > 0 {
			fmt.Fprintf(b, "          printf 'replicaof %s 6379\\n'\n", primary)
		}
		b.WriteString("        } > /tmp/valkey.conf\n")
		b.WriteString("        exec valkey-server /tmp/valkey.conf\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(b, "      - %s:/data\n", sharedValkeyMemberVolumeName(app, instance, ordinal))
		b.WriteString("    networks:\n      shared-backend: {}\n\n")
	}
	if count > 1 {
		for ordinal := 0; ordinal < 3; ordinal++ {
			service := sharedValkeySentinelServiceName(app, instance, ordinal)
			fmt.Fprintf(b, "  %s:\n", service)
			b.WriteString("    image: docker.io/valkey/valkey:9.1.2-alpine\n")
			b.WriteString("    restart: unless-stopped\n")
			b.WriteString("    user: \"999:1000\"\n")
			b.WriteString("    read_only: true\n")
			b.WriteString("    cap_drop: [\"ALL\"]\n")
			b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
			b.WriteString("    tmpfs:\n      - /tmp:rw,noexec,nosuid,nodev\n")
			b.WriteString("    environment:\n")
			fmt.Fprintf(b, "      VALKEY_PASSWORD: ${%s}\n", passwordEnv)
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
			b.WriteString("    networks:\n      shared-backend: {}\n\n")
		}
	}
	gatewayFiles := serviceaccess.TCPGatewayFiles{
		Config:   root + "/service-access/haproxy.cfg",
		PEM:      root + "/service-access/runtime/server.pem",
		Material: serviceaccess.TLSMaterial{},
	}
	b.WriteString(serviceaccess.TCPGatewayComposeService(gatewayFiles, sharedValkeyGatewaySpec(app, instance)))
}
