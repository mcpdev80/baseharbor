package main

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/health"
	"strings"
	"testing"
)

func TestTUICoreTabsNavigateDistinctBodies(t *testing.T) {
	m := tuiModel{coreMode: true, noColor: true, width: 100, coreView: "overview-only", coreStatus: "status-only", coreDoctor: "doctor-only"}
	for i, want := range []string{"overview-only", "status-only", "doctor-only", "overview-only"} {
		view := m.View().Content
		if !strings.Contains(view, want) {
			t.Fatalf("tab %d missing %q: %s", i, want, view)
		}
		for _, other := range []string{"overview-only", "status-only", "doctor-only"} {
			if other != want && strings.Contains(view, other) {
				t.Fatalf("tab leaked %s", other)
			}
		}
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		m = updated.(tuiModel)
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = updated.(tuiModel)
	if m.tab != 0 {
		t.Fatalf("left navigation: %d", m.tab)
	}
}

func TestTUITabsLoadingErrorAndRefresh(t *testing.T) {
	for _, core := range []bool{false, true} {
		for tab := 0; tab < 3; tab++ {
			m := tuiModel{coreMode: core, tab: tab, noColor: true, loading: true, reducedMotion: true}
			if !strings.Contains(m.View().Content, "Refreshing status...") {
				t.Fatal("missing loading view")
			}
			updated, _ := m.Update(tuiStatusMsg{coreView: "overview", coreStatus: "status", coreDoctor: "diagnosis", err: errors.New("unavailable")})
			m = updated.(tuiModel)
			if m.loading || !strings.Contains(m.View().Content, "FAILED") {
				t.Fatal("missing error view")
			}
			updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r'})
			m = updated.(tuiModel)
			if !m.loading || m.err != nil || cmd == nil {
				t.Fatal("refresh did not reset error")
			}
		}
	}
}

func TestTUIAppTabsStayDistinct(t *testing.T) {
	m := tuiModel{noColor: true, result: application.StatusResult{Application: "example", Project: "app-project", Ready: true}}
	m.tab = 0
	summary := m.View().Content
	m.tab = 1
	status := m.View().Content
	m.tab = 2
	doctor := m.View().Content
	if !strings.Contains(summary, "app-project") || strings.Contains(status, "app-project") || !strings.Contains(doctor, "No doctor checks") {
		t.Fatal("app tabs regressed")
	}
}

func TestCoreTUIDoctorReusesFindingActions(t *testing.T) {
	text := renderCoreTUIDoctor([]health.Check{{Name: "target-selection", OK: false, Message: "select a target"}})
	for _, want := range []string{"FAILED", "target-selection", "baha target activate NAME"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s: %s", want, text)
		}
	}
	if strings.Contains(text, "baha app doctor") {
		t.Fatal("Core doctor suggested app repair")
	}
}

func TestTUICoreNoTargetLoadsEveryView(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("BASEHARBOR_TARGET", "")
	m := tuiModel{ctx: context.Background(), coreMode: true}
	msg := m.loadStatus()().(tuiStatusMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	if msg.coreView == "" || msg.coreStatus == "" || msg.coreDoctor == "" || msg.coreView == msg.coreStatus || msg.coreStatus == msg.coreDoctor {
		t.Fatalf("missing distinct results: %+v", msg)
	}
}

func TestTUICoreOverviewInvokesSelectedTargetRuntimeInventory(t *testing.T) {
	target := configureTestTarget(t)
	t.Setenv("PATH", t.TempDir()) // No engine: require truthful unavailable telemetry.
	m := tuiModel{ctx: context.Background(), coreMode: true}
	msg := m.loadStatus()().(tuiStatusMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	for _, want := range []string{target.Name, "Current Device", "Runtime resources (selected Target)", "Runtime Explorer unavailable"} {
		if !strings.Contains(msg.coreView, want) {
			t.Fatalf("missing actual inventory view %q: %s", want, msg.coreView)
		}
	}
}
