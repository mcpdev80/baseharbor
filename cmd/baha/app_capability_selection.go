package main

import (
	"fmt"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

// appCapabilityNeed describes portable intent, never a chosen provider.
// The ordering keeps everyday application needs on the first screen.
type appCapabilityNeed struct {
	Kind        capability.Kind
	Label       string
	Description string
	Extended    bool
}

var appCapabilityNeeds = []appCapabilityNeed{
	{capability.SQL, "SQL database", "Store relational application data", false},
	{capability.KeyValue, "Cache", "Cache temporary application data", false},
	{capability.Secrets, "Protected application secrets", "Store application secret values (not service credentials)", false},
	{capability.Identity, "Application login", "Sign in application end users via OIDC", false},
	{capability.ExposureHTTP, "TLS / HTTPS", "Expose the application over HTTP with TLS", false},
	{capability.ObjectStorageS3, "Object storage", "Store files and objects", true},
	{capability.DurableKeyValue, "Durable key-value database", "Persist key-value records", true},
	{capability.DocumentDatabase, "Document database", "Store documents", true},
	{capability.MessagingQueue, "Message queue", "Process asynchronous jobs", true},
	{capability.MessagingPubSub, "Message publish/subscribe", "Distribute messages", true},
	{capability.MessagingStream, "Message stream", "Consume ordered event streams", true},
	{capability.TelemetryOTLP, "OpenTelemetry", "Export application telemetry", true},
	{capability.Metrics, "Metrics", "Collect metrics", true},
	{capability.Logs, "Logs", "Collect logs", true},
	{capability.Traces, "Traces", "Collect distributed traces", true},
}

// appCapabilitySelectionInput makes detected suggestions explicit, not mandatory.
type appCapabilitySelectionInput struct {
	Current  []capability.Requirement
	Detected []capability.Kind
}

// appCapabilitySelectionModel is an independent read-only-until-confirmed TUI
// component. No file, registry or provider mutation occurs in Update.
type appCapabilitySelectionModel struct {
	selected map[capability.Kind]bool
	detected map[capability.Kind]bool
	cursor   int
	extended bool
	done     bool
}

func newAppCapabilitySelection(input appCapabilitySelectionInput) appCapabilitySelectionModel {
	m := appCapabilitySelectionModel{
		selected: make(map[capability.Kind]bool),
		detected: make(map[capability.Kind]bool),
	}
	for _, req := range input.Current {
		if appCapabilityNeedSupported(req.Kind) {
			m.selected[req.Kind] = true
		}
	}
	for _, kind := range input.Detected {
		if appCapabilityNeedSupported(kind) {
			m.detected[kind] = true
			// Suggestions are initially selected but remain freely deselectable.
			m.selected[kind] = true
		}
	}
	for _, need := range appCapabilityNeeds {
		if need.Extended && m.selected[need.Kind] {
			m.extended = true
		}
	}
	return m
}

func appCapabilityNeedSupported(kind capability.Kind) bool {
	for _, need := range appCapabilityNeeds {
		if need.Kind == kind {
			return true
		}
	}
	return false
}

func (m appCapabilitySelectionModel) visibleNeeds() []appCapabilityNeed {
	needs := make([]appCapabilityNeed, 0, len(appCapabilityNeeds))
	for _, need := range appCapabilityNeeds {
		if !need.Extended || m.extended {
			needs = append(needs, need)
		}
	}
	return needs
}

// Update accepts key names as produced by Bubble Tea (up, down, space,
// enter, esc and ctrl+c). The return value signals confirmation only.
func (m appCapabilitySelectionModel) Update(key string) (appCapabilitySelectionModel, bool) {
	if m.done {
		return m, true
	}
	switch strings.ToLower(key) {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.visibleNeeds()) {
			m.cursor++
		}
	case " ", "space":
		needs := m.visibleNeeds()
		if m.cursor == len(needs) {
			m.extended = !m.extended
			if !m.extended && m.cursor > len(m.visibleNeeds()) {
				m.cursor = len(m.visibleNeeds())
			}
		} else if m.cursor >= 0 && m.cursor < len(needs) {
			kind := needs[m.cursor].Kind
			m.selected[kind] = !m.selected[kind]
		}
	case "enter":
		if m.cursor == len(m.visibleNeeds()) {
			m.extended = !m.extended
		} else {
			m.done = true
			return m, true
		}
	}
	return m, false
}

// Requirements emits only selected portable kinds in catalog order.
// A zero-selection result is valid; nothing is implicitly provisioned.
func (m appCapabilitySelectionModel) Requirements() []capability.Requirement {
	var result []capability.Requirement
	for _, need := range appCapabilityNeeds {
		if m.selected[need.Kind] {
			result = append(result, capability.Requirement{Kind: need.Kind, Name: "default"})
		}
	}
	return result
}

func (m appCapabilitySelectionModel) View() string {
	var b strings.Builder
	b.WriteString("What does your application need?\n")
	b.WriteString("↑/↓ move · Space toggle · Enter confirm · no selection is OK\n\n")
	needs := m.visibleNeeds()
	for i, need := range needs {
		pointer := " "
		if i == m.cursor {
			pointer = ">"
		}
		check := " "
		if m.selected[need.Kind] {
			check = "x"
		}
		suggested := ""
		if m.detected[need.Kind] {
			suggested = " (detected suggestion)"
		}
		fmt.Fprintf(&b, "%s [%s] %s%s — %s\n", pointer, check, need.Label, suggested, need.Description)
	}
	pointer := " "
	if m.cursor == len(needs) {
		pointer = ">"
	}
	if m.extended {
		fmt.Fprintf(&b, "%s Show fewer needs\n", pointer)
	} else {
		fmt.Fprintf(&b, "%s Show more supported needs…\n", pointer)
	}
	b.WriteString("\nApplication login is for end users; operator/team access and service credentials are separate settings.\n")
	b.WriteString("No SQL, identity service, secrets service or management stack is mandatory.\n")
	return b.String()
}
