package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
)

func TestQuickInitNonTTYDoesNotRequireCore(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(httpsAdoptionFixture(t, false))
	old := appInitInput
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { input.Close() })
	appInitInput = input
	t.Cleanup(func() { appInitInput = old })
	var out bytes.Buffer
	err = runWithIO(context.Background(), []string{"app", "init", "--quick"}, &out, io.Discard)
	if err != nil || strings.Contains(out.String(), "Set them up now?") {
		t.Fatalf("unexpected Core prerequisite: %v %s", err, out.String())
	}
	if _, err := os.Stat("baseharbor.yaml"); err != nil {
		t.Fatal("authored intent was not initialized")
	}
}

func TestQuickInitYesDoesNotAuthorizeUnrequestedCore(t *testing.T) {
	for _, flag := range []string{"--yes", "-y"} {
		t.Run(flag, func(t *testing.T) {
			configureTestTarget(t)
			t.Chdir(httpsAdoptionFixture(t, false))
			old, oldInput := applicationCoreBootstrap, appInitInput
			t.Cleanup(func() { applicationCoreBootstrap, appInitInput = old, oldInput })
			appInitInput = interactiveCoreInput(t, "")
			called := false
			failure := errors.New("bootstrap blocked safely")
			applicationCoreBootstrap = func(ctx context.Context, in io.Reader, out io.Writer, opts runtimeUpOptions) (coreinstallation.State, error) {
				called = opts.Yes
				return coreinstallation.State{}, failure
			}
			err := runWithIO(context.Background(), []string{"app", "init", "--quick", flag}, io.Discard, io.Discard)
			if called || err != nil {
				t.Fatalf("unrequested Core bootstrap: %v %v", called, err)
			}
			if _, err := os.Stat("baseharbor.yaml"); err != nil {
				t.Fatal("authored intent was not initialized")
			}
		})
	}
}
