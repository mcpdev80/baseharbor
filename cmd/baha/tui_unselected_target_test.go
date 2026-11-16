package main

import (
	"errors"
	"strings"
	"testing"
)

func TestTargetlessTUIKeepsCurrentDeviceAndRemediation(t *testing.T) {
	view := renderUnselectedCoreTUIView("HOST: 16 GiB", "Targets: core-a, core-b", errors.New("multiple targets need selection"))
	for _, wanted := range []string{"HOST: 16 GiB", "Targets: core-a, core-b", "multiple targets need selection", "baha target list", "baha target activate NAME"} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("targetless TUI lost %q: %s", wanted, view)
		}
	}
}
