package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

func TestRunBestEffortDevelopmentRouteSuspensionDoesNotBlockDown(t *testing.T) {
	var out bytes.Buffer
	term := cli.NewTerminal(context.Background(), &out, &out)

	called := false
	runBestEffortDevelopmentRouteSuspension(term, func() error {
		called = true
		return errors.New("OpenBao AppRole login failed: service \"openbao\" is not running")
	})

	if !called {
		t.Fatal("route suspension callback was not called")
	}
	text := out.String()
	if !strings.Contains(text, "WARN") {
		t.Fatalf("warning missing: %s", text)
	}
	if !strings.Contains(text, "development-routes") {
		t.Fatalf("warning subject missing: %s", text)
	}
	if !strings.Contains(text, "application shutdown will continue") {
		t.Fatalf("continuation guidance missing: %s", text)
	}
	if !strings.Contains(text, "openbao") {
		t.Fatalf("original route reconciliation failure missing: %s", text)
	}
}

func TestRunBestEffortDevelopmentRouteSuspensionHealthyIsSilent(t *testing.T) {
	var out bytes.Buffer
	term := cli.NewTerminal(context.Background(), &out, &out)

	runBestEffortDevelopmentRouteSuspension(term, func() error { return nil })

	if out.Len() != 0 {
		t.Fatalf("healthy route suspension produced output: %q", out.String())
	}
}
