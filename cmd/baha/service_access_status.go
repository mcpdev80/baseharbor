package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

func serviceTLSLifecycleHealthy(items []application.BackendTLSLifecycleObservation) bool {
	for _, item := range items {
		if item.Lifecycle.Health == "critical" || item.Lifecycle.Health == "unknown" {
			return false
		}
	}
	return true
}

func renderServiceTLSLifecycle(term *cli.Terminal, items []application.BackendTLSLifecycleObservation) {
	if len(items) == 0 {
		return
	}
	term.Section("Service TLS")
	for _, item := range items {
		label := item.Kind
		if strings.TrimSpace(item.Instance) != "" {
			label += "/" + item.Instance
		}
		state := "READY"
		switch item.Lifecycle.Health {
		case "warn":
			state = "WARN"
		case "critical", "unknown":
			state = "FAILED"
		}
		detail := fmt.Sprintf(
			"%s · owner=%s · renewal=%s · expires=%s",
			item.Lifecycle.Source,
			item.Lifecycle.LifecycleOwner,
			item.Lifecycle.RenewalMode,
			formatServiceTLSExpiry(item.Lifecycle.ServerExpiresAt),
		)
		if item.Lifecycle.Warning != "" {
			detail += " · " + item.Lifecycle.Warning
		}
		term.Result(state, label, detail)
		if term.Verbose() && item.Lifecycle.IssuerReference != "" {
			term.Info(label+" issuer", item.Lifecycle.IssuerReference)
		}
	}
}

func formatServiceTLSExpiry(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.Local().Format("2006-01-02 15:04 MST")
}
