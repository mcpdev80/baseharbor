package identityprovider

// The non-HA Core topology uses one PostgreSQL server with a durable data
// volume. It deliberately does not create Patroni/etcd or HAProxy services.
func keycloakSingleDataLayerCompose() string {
	return `  keycloak-db:
    image: docker.io/library/postgres:18-alpine
    restart: unless-stopped
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD}
      POSTGRES_DB: postgres
      PGDATA: /var/lib/postgresql/18/docker
    command:
      - /bin/sh
      - -ec
      - |
        cp /run/baseharbor/db-tls/server-key.pem /tmp/server-key.pem
        chmod 0600 /tmp/server-key.pem
        if [ "$$(id -u)" = 0 ]; then chown postgres:postgres /tmp/server-key.pem; fi
        exec /usr/local/bin/docker-entrypoint.sh postgres -c ssl=on -c ssl_cert_file=/run/baseharbor/db-tls/server.pem -c ssl_key_file=/tmp/server-key.pem
    volumes:
      - keycloak-db-data:/var/lib/postgresql
      - ./db-ha/runtime/ca.pem:/run/baseharbor/db-tls/ca.pem:ro
      - ./db-ha/runtime/server.pem:/run/baseharbor/db-tls/server.pem:ro
      - ./db-ha/runtime/server-key.pem:/run/baseharbor/db-tls/server-key.pem:ro
    tmpfs:
      - /tmp:rw,noexec,nosuid,nodev,mode=1777
      - /var/run/postgresql:rw,noexec,nosuid,nodev,mode=3777
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d postgres"]
      interval: 2s
      timeout: 3s
      retries: 90
    networks:
      identity-internal: {}

` + keycloakDBInitCompose()
}

func keycloakSingleVolumesCompose() string {
	return "  keycloak-db-data:\n"
}

// The shared bootstrap is deliberately identical for single and HA database
// realizations. It creates the least-privileged application login only after
// the selected SQL endpoint has become writable and TLS verified.
func keycloakDBInitCompose() string {
	return `  keycloak-db-init:
    image: docker.io/library/postgres:18-alpine
    restart: "no"
    read_only: true
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    depends_on:
      keycloak-db:
        condition: service_started
    environment:
      PGPASSWORD: ${BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD}
      BASEHARBOR_KEYCLOAK_DB_USER: ${BASEHARBOR_KEYCLOAK_DB_USER}
      BASEHARBOR_KEYCLOAK_DB_PASSWORD: ${BASEHARBOR_KEYCLOAK_DB_PASSWORD}
      BASEHARBOR_KEYCLOAK_DB_NAME: ${BASEHARBOR_KEYCLOAK_DB_NAME}
      PGSSLMODE: verify-full
      PGSSLROOTCERT: /run/baseharbor/db-tls/ca.pem
      PGCONNECT_TIMEOUT: "1"
    volumes:
      - ./db-ha/runtime/ca.pem:/run/baseharbor/db-tls/ca.pem:ro
    command:
      - /bin/sh
      - -ec
      - |
        attempts=0; until psql -h keycloak-db -p 5432 -U postgres -d postgres -v ON_ERROR_STOP=1 -c 'SELECT 1' >/dev/null; do attempts=$$((attempts+1)); if [ "$$attempts" -ge 90 ]; then echo 'Keycloak PostgreSQL did not become ready' >&2; exit 1; fi; sleep 1; done
        role_exists=$$(printf '%s\n' "SELECT 1 FROM pg_roles WHERE rolname = :'app_user';" | psql -h keycloak-db -U postgres -d postgres -v app_user="$$BASEHARBOR_KEYCLOAK_DB_USER" -tA)
        if [ "$$role_exists" != "1" ]; then printf '%s\n' "SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'app_user', :'app_password');" | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1 -v app_user="$$BASEHARBOR_KEYCLOAK_DB_USER" -v app_password="$$BASEHARBOR_KEYCLOAK_DB_PASSWORD" -At | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1; else printf '%s\n' "SELECT format('ALTER ROLE %I WITH LOGIN PASSWORD %L', :'app_user', :'app_password');" | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1 -v app_user="$$BASEHARBOR_KEYCLOAK_DB_USER" -v app_password="$$BASEHARBOR_KEYCLOAK_DB_PASSWORD" -At | psql -h keycloak-db -U postgres -d postgres -v ON_ERROR_STOP=1; fi
        exists=$$(printf '%s\n' "SELECT 1 FROM pg_database WHERE datname = :'db_name';" | psql -h keycloak-db -U postgres -d postgres -v db_name="$$BASEHARBOR_KEYCLOAK_DB_NAME" -tA)
        if [ "$$exists" != "1" ]; then createdb -h keycloak-db -U postgres -O "$$BASEHARBOR_KEYCLOAK_DB_USER" "$$BASEHARBOR_KEYCLOAK_DB_NAME"; fi
    networks:
      identity-internal: {}

`
}
