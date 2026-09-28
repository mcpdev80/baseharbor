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

	for _, instance := range CacheInstanceNames(m) {
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
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
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
			"-h", "127.0.0.1",
			"-p", "5432",
			"-U", "baseharbor_admin",
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
			if lastErr != nil {
				return fmt.Errorf("wait for shared PostgreSQL readiness: %w", lastErr)
			}
			return fmt.Errorf("wait for shared PostgreSQL readiness: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
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
		if _, err := compose.ExecProjectInput(ctx, shared.Project, shared.Compose, shared.Env, []byte(sql), sharedPostgresService(app.Environment), "psql", "-U", "baseharbor_admin", "-d", "postgres", "-v", "ON_ERROR_STOP=1"); err != nil {
			return fmt.Errorf("reconcile shared PostgreSQL resource %s: %w", instance, err)
		}
		harden := fmt.Sprintf("REVOKE ALL ON SCHEMA public FROM PUBLIC; GRANT USAGE, CREATE ON SCHEMA public TO %s; REVOKE ALL ON ALL TABLES IN SCHEMA public FROM PUBLIC; REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC; REVOKE ALL ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON TABLES FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON SEQUENCES FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON FUNCTIONS FROM PUBLIC; ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA public REVOKE ALL ON TYPES FROM PUBLIC;",
			quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username), quotePostgresIdent(resource.Username))
		if _, err := compose.ExecProject(ctx, shared.Project, shared.Compose, shared.Env, sharedPostgresService(app.Environment), "psql", "-U", "baseharbor_admin", "-d", resource.Database, "-v", "ON_ERROR_STOP=1", "-c", harden); err != nil {
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
	hasPostgres := false
	for _, app := range state.Applications {
		if len(app.SQL) > 0 {
			hasPostgres = true
			break
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
		fmt.Fprintf(&b, "  shared-postgres-data:\n    name: %s-%s-postgres-data\n", files.ResourceProject, sharedBackendToken(state.Environment))
	}
	for _, key := range appKeys {
		app := state.Applications[key]
		for instance := range app.Cache {
			service := sharedValkeyServiceFor(app.Application, app.Environment, instance)
			fmt.Fprintf(&b, "  %s-data:\n    name: %s-%s-%s-data\n", service, files.ResourceProject, sharedBackendToken(state.Environment), service)
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
	root := "./postgresql/runtime"
	fmt.Fprintf(b, `  %s:
    image: docker.io/library/postgres:18-alpine
    restart: unless-stopped
    user: "postgres"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    entrypoint: ["/bin/sh", "-ec"]
    command:
      - |
        cp /run/baseharbor/tls-source/server-key.pem /tmp/server-key.pem
        chmod 0600 /tmp/server-key.pem
        exec /usr/local/bin/docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/run/baseharbor/tls-source/server-cert.pem -c ssl_key_file=/tmp/server-key.pem -c hba_file=/run/baseharbor/tls-source/pg_hba.conf
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
      - /var/run/postgresql:rw,noexec,nosuid,nodev
    environment:
      POSTGRES_DB: postgres
      POSTGRES_USER: baseharbor_admin
      POSTGRES_PASSWORD: ${SHARED_POSTGRES_ADMIN_PASSWORD}
    ports:
      - "127.0.0.1:${SHARED_POSTGRES_HOST_PORT}:5432"
    volumes:
      - shared-postgres-data:/var/lib/postgresql
      - %s/server-cert.pem:/run/baseharbor/tls-source/server-cert.pem:ro
      - %s/server-key.pem:/run/baseharbor/tls-source/server-key.pem:ro
      - %s/pg_hba.conf:/run/baseharbor/tls-source/pg_hba.conf:ro
    networks:
      shared-backend:
        aliases:
          - %s
    healthcheck:
      test: ["CMD", "pg_isready", "-h", "127.0.0.1", "-p", "5432", "-U", "baseharbor_admin", "-d", "postgres"]
      interval: 5s
      timeout: 5s
      retries: 12
      start_period: 5s

`, service, root, root, root, sharedPostgresAlias())
}

func writeSharedValkeyCompose(b *strings.Builder, app sharedBackendAppState, instance string) {
	service := sharedValkeyServiceFor(app.Application, app.Environment, instance)
	access := sharedValkeyAccessServiceFor(app.Application, app.Environment, instance)
	passwordEnv := sharedValkeyPasswordEnvFor(app.Application, app.Environment, instance)
	portEnv := sharedValkeyPortEnvFor(app.Application, app.Environment, instance)
	root := "./" + filepath.ToSlash(filepath.Join("valkey", sharedBackendToken(app.Application), sharedBackendToken(instance)))
	fmt.Fprintf(b, `  %s:
    image: docker.io/valkey/valkey:9.1.2-alpine
    restart: unless-stopped
    user: "valkey"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev
    environment:
      VALKEY_PASSWORD: ${%s}
    command:
      - sh
      - -ec
      - |
        printf 'requirepass %%s\nappendonly yes\ndir /data\n' "$$VALKEY_PASSWORD" > /tmp/valkey.conf
        exec valkey-server /tmp/valkey.conf
    volumes:
      - %s-data:/data
    networks:
      shared-backend: {}

`, service, passwordEnv, service)
	gatewayFiles := serviceaccess.TCPGatewayFiles{
		Config:   root + "/service-access/haproxy.cfg",
		PEM:      root + "/service-access/runtime/server.pem",
		Material: serviceaccess.TLSMaterial{},
	}
	b.WriteString(serviceaccess.TCPGatewayComposeService(gatewayFiles, serviceaccess.TCPGatewaySpec{
		ServiceName:      access,
		UpstreamHost:     service,
		UpstreamPort:     6379,
		PublishedPortEnv: portEnv,
		ContainerPort:    6379,
		Network:          "shared-backend",
	}))
}
