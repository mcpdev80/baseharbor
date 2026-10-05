package application

import (
	"fmt"
	"strings"
)

func writeSharedPostgresSingleCompose(b *strings.Builder, state sharedBackendState) {
	fmt.Fprintf(b, `  %s:
    image: docker.io/library/postgres:18-alpine
    restart: unless-stopped
    user: "70:70"
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
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${SHARED_POSTGRES_SUPERUSER_PASSWORD}
      SHARED_POSTGRES_ADMIN_PASSWORD: ${SHARED_POSTGRES_ADMIN_PASSWORD}
      SHARED_POSTGRES_SUPERUSER_PASSWORD: ${SHARED_POSTGRES_SUPERUSER_PASSWORD}
      PGPASSWORD: ${SHARED_POSTGRES_ADMIN_PASSWORD}
    ports:
      - "127.0.0.1:${SHARED_POSTGRES_HOST_PORT}:5432"
    volumes:
      - shared-postgres-data-1:/var/lib/postgresql
      - ./postgresql/runtime/server-cert.pem:/run/baseharbor/tls-source/server-cert.pem:ro
      - ./postgresql/runtime/server-key.pem:/run/baseharbor/tls-source/server-key.pem:ro
      - ./postgresql/runtime/pg_hba.conf:/run/baseharbor/tls-source/pg_hba.conf:ro
    networks:
      shared-backend:
        aliases:
          - postgres-access
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -h 127.0.0.1 -p 5432 -U postgres -d postgres"]
      interval: 5s
      timeout: 5s
      retries: 24
      start_period: 10s

`, sharedPostgresService(state.Environment))
}
