package application

import (
	"strings"
	"testing"
)

func TestMongoDBManagementProxyWatchesProjectedLoginConfiguration(t *testing.T) {
	var config strings.Builder
	writeMongoDBUIComposeServices(&config, New("demo", "dev", false, false, false), "default")
	if !strings.Contains(config.String(), "caddy run --config /etc/caddy/Caddyfile --adapter caddyfile --watch") {
		t.Fatal("MongoDB UI proxy would keep stale management credentials after apply")
	}
	if !strings.Contains(config.String(), "Caddyfile:/etc/caddy/Caddyfile:ro") {
		t.Fatal("management config must remain a read-only consumer projection")
	}
}
