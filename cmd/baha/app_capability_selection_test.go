package main

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestAppCapabilitySelectionZeroIsValid(t *testing.T) {
	m := newAppCapabilitySelection(appCapabilitySelectionInput{})
	if got := m.Requirements(); len(got) != 0 {
		t.Fatalf("empty selection must not provision services: %v", got)
	}
	if strings.Contains(m.View(), "[x]") {
		t.Fatal("default selection must be empty")
	}
	updated, done := m.Update("enter")
	if !done || !updated.done || len(updated.Requirements()) != 0 {
		t.Fatal("enter should confirm empty intent")
	}
}

func TestAppCapabilitySelectionKeyboardNavigationAndToggle(t *testing.T) {
	m := newAppCapabilitySelection(appCapabilitySelectionInput{})
	m, _ = m.Update("down")
	m, _ = m.Update("space")
	got := m.Requirements()
	if len(got) != 1 || got[0].Kind != capability.KeyValue {
		t.Fatalf("expected cache only: %+v", got)
	}
	m, _ = m.Update("up")
	m, _ = m.Update(" ")
	m, _ = m.Update("down")
	m, _ = m.Update("space")
	got = m.Requirements()
	if len(got) != 1 || got[0].Kind != capability.SQL {
		t.Fatalf("toggle must be reversible: %+v", got)
	}
	m, done := m.Update("enter")
	if !done {
		t.Fatal("enter must confirm")
	}
	before := m.Requirements()
	m, _ = m.Update("space")
	if !reflect.DeepEqual(before, m.Requirements()) {
		t.Fatal("confirmed selection must be immutable")
	}
}

func TestAppCapabilitySelectionDetectedSuggestionsCanBeDeselected(t *testing.T) {
	m := newAppCapabilitySelection(appCapabilitySelectionInput{
		Detected: []capability.Kind{capability.SQL, capability.Secrets, capability.SQL},
	})
	if len(m.Requirements()) != 2 {
		t.Fatalf("detected suggestions should be unique: %+v", m.Requirements())
	}
	if !strings.Contains(m.View(), "SQL database (detected suggestion)") {
		t.Fatal("detected suggestion must be visible")
	}
	m, _ = m.Update("space")
	for i := 0; i < 2; i++ {
		m, _ = m.Update("down")
	}
	m, _ = m.Update("space")
	if got := m.Requirements(); len(got) != 0 {
		t.Fatalf("detected suggestions must be deselectable: %+v", got)
	}
}

func TestAppCapabilitySelectionExtendedNeedsAreProgressive(t *testing.T) {
	m := newAppCapabilitySelection(appCapabilitySelectionInput{})
	if strings.Contains(m.View(), "Message queue") {
		t.Fatal("extended capabilities should start hidden")
	}
	for i := 0; i < 5; i++ {
		m, _ = m.Update("down")
	}
	m, done := m.Update("enter")
	if done || !m.extended || !strings.Contains(m.View(), "Message queue") {
		t.Fatal("more needs must expand without confirming")
	}
	m, _ = m.Update("space")
	if got := m.Requirements(); len(got) != 1 || got[0].Kind != capability.ObjectStorageS3 {
		t.Fatalf("expected object storage extension: %+v", got)
	}
}

func TestAppCapabilitySelectionCurrentExtendedStateAndCatalog(t *testing.T) {
	m := newAppCapabilitySelection(appCapabilitySelectionInput{
		Current:  []capability.Requirement{{Kind: capability.MessagingStream, Name: "orders"}},
		Detected: []capability.Kind{"unsupported.capability"},
	})
	if !m.extended || !strings.Contains(m.View(), "Message stream") {
		t.Fatal("current extended capability must be visible")
	}
	if got := m.Requirements(); len(got) != 1 || got[0].Kind != capability.MessagingStream {
		t.Fatalf("selection must use supported catalog kinds: %+v", got)
	}
	for _, req := range m.Requirements() {
		if !appCapabilityNeedSupported(req.Kind) {
			t.Fatalf("unsupported kind: %s", req.Kind)
		}
	}
}

func TestAppCapabilitySelectionWordingSeparatesAccessFromSecrets(t *testing.T) {
	view := newAppCapabilitySelection(appCapabilitySelectionInput{}).View()
	for _, phrase := range []string{
		"Application login", "end users", "operator/team access",
		"service credentials", "Protected application secrets",
		"TLS / HTTPS", "No SQL, identity service",
	} {
		if !strings.Contains(view, phrase) {
			t.Fatalf("view lacks %q: %s", phrase, view)
		}
	}
	if strings.Contains(view, "Keycloak") || strings.Contains(view, "OpenBao") {
		t.Fatal("selection must describe needs, not implementation providers")
	}
}

func TestAppCapabilitySelectionCursorBounds(t *testing.T) {
	m := newAppCapabilitySelection(appCapabilitySelectionInput{})
	m, _ = m.Update("up")
	if m.cursor != 0 {
		t.Fatal("cursor moved outside upper bound")
	}
	for i := 0; i < 100; i++ {
		m, _ = m.Update("down")
	}
	if m.cursor != len(m.visibleNeeds()) {
		t.Fatalf("cursor outside lower bound: %d", m.cursor)
	}
	m, _ = m.Update("space")
	if !m.extended {
		t.Fatal("expected expansion on footer")
	}
	for i := 0; i < 100; i++ {
		m, _ = m.Update("down")
	}
	m, _ = m.Update("space")
	if m.extended || m.cursor != len(m.visibleNeeds()) {
		t.Fatal("collapse should clamp cursor to footer")
	}
}

func TestAppCapabilitySelectionFormatting(t *testing.T) {
	for _, file := range []string{"app_capability_selection.go", "app_capability_selection_test.go"} {
		output, err := exec.Command("gofmt", "-d", file).CombinedOutput()
		if err != nil || len(output) != 0 {
			t.Fatalf("formatting mismatch in %s: %s (%v)", file, output, err)
		}
	}
}
