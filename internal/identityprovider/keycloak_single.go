package identityprovider

// The non-HA Core topology uses one PostgreSQL server with a durable data
// volume. It deliberately does not create Patroni/etcd or HAProxy services.
func keycloakSingleDataLayerCompose() string {
	return `  keycloak-db-tls-init:
    image: docker.io/library/postgres:18-alpine
    restart: "no"
    user: "0:0"
    entrypoint: ["/bin/sh", "-ec"]
    command:
      - |
        uid=$$(id -u postgres); gid=$$(id -g postgres)
        cp /source/ca.pem /target/ca.pem
        cp /source/server.pem /target/server.pem
        cp /source/server-key.pem /target/server-key.pem
        chown "$$uid:$$gid" /target/ca.pem /target/server.pem /target/server-key.pem
        chmod 0644 /target/ca.pem /target/server.pem
        chmod 0600 /target/server-key.pem
    volumes:
      - ./db-ha/runtime:/source:ro
      - keycloak-db-tls:/target
    networks:
      identity-internal: {}

  keycloak-db:
    image: docker.io/library/postgres:18-alpine
    restart: unless-stopped
    cap_drop: ["ALL"]
    security_opt: ["no-new-privileges:true"]
    depends_on:
      keycloak-db-tls-init:
        condition: service_completed_successfully
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD}
      POSTGRES_DB: postgres
    command:
      - postgres
      - -c
      - ssl=on
      - -c
      - ssl_cert_file=/run/baseharbor/db-tls/server.pem
      - -c
      - ssl_key_file=/run/baseharbor/db-tls/server-key.pem
    volumes:
      - keycloak-db-data:/var/lib/postgresql
      - keycloak-db-tls:/run/baseharbor/db-tls:ro
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
	return "  keycloak-db-data:\n  keycloak-db-tls:\n"
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
      keycloak-db-tls-init:
        condition: service_completed_successfully
    environment:
      PGPASSWORD: ${BASEHARBOR_KEYCLOAK_DB_SUPERUSER_PASSWORD}
      BASEHARBOR_KEYCLOAK_DB_USER: ${BASEHARBOR_KEYCLOAK_DB_USER}
      BASEHARBOR_KEYCLOAK_DB_PASSWORD: ${BASEHARBOR_KEYCLOAK_DB_PASSWORD}
      BASEHARBOR_KEYCLOAK_DB_NAME: ${BASEHARBOR_KEYCLOAK_DB_NAME}
      PGSSLMODE: verify-full
      PGSSLROOTCERT: /run/baseharbor/db-tls/ca.pem
      PGCONNECT_TIMEOUT: "1"
    volumes:
      - keycloak-db-tls:/run/baseharbor/db-tls:ro
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
