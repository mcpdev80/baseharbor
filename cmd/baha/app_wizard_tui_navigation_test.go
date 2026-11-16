package main

import (
	"context"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/capability"
)

func TestNeedsWizardBackNavigationMovesToPreviousRow(t *testing.T) {
	model := newAppCapabilitySelection(appCapabilitySelectionInput{})
	model, _ = model.Update("down")
	if model.cursor != 1 {
		t.Fatalf("setup cursor = %d", model.cursor)
	}
	model, _ = model.Update("b")
	if model.cursor != 0 {
		t.Fatalf("back did not move to previous row: %d", model.cursor)
	}
	if !strings.Contains(model.View(), "b/← previous row") {
		t.Fatalf("back navigation is not visible: %s", model.View())
	}
}

func TestWizardSelectionDisplaysProviderResolutionError(t *testing.T) {
	controller, err := newAppWizardController(context.Background(), appWizardTestInput(t))
	if err != nil {
		t.Fatal(err)
	}
	req := capability.Requirement{Kind: capability.SQL, Name: "primary"}
	controller.preferences[appWizardKey(controller.snapshot.Target, req)] = "does-not-exist"
	if _, err := controller.choicesForSelection(); err == nil {
		t.Fatal("expected an invalid provider preference to be rejected")
	}
	display := controller.View()
	if !strings.Contains(display, "Provider choices unavailable:") ||
		!strings.Contains(display, "does-not-exist") ||
		!strings.Contains(display, "no choice was confirmed") {
		t.Fatalf("provider resolution error was hidden: %s", display)
	}
}

func TestProviderDisplayWrapDoesNotOverflowLongReasons(t *testing.T) {
	long := "Selected: " + strings.Repeat("long reason ", 20)
	display := wrapWizardDisplay(long, 80)
	for _, line := range strings.Split(display, "\n") {
		if len([]rune(line)) > 80 {
			t.Fatalf("provider display line exceeds width: %q", line)
		}
	}
}
