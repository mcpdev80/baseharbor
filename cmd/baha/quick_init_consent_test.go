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
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestQuickInitNonTTYRequiresCoreWithoutPromptOrMutation(t *testing.T) {
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
	var typed *machine.Error
	if !errors.As(err, &typed) || typed.CauseCode != "core_required" || strings.Contains(out.String(), "Set them up now?") {
		t.Fatalf("non-TTY prerequisite: %v %s", err, out.String())
	}
	if _, err := os.Stat("baseharbor.yaml"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("wrote manifest without Core")
	}
}

func TestQuickInitYesAcceptsConsentButKeepsBootstrapFailureAtomic(t *testing.T) {
	for _, flag := range []string{"--yes", "-y"} {
		t.Run(flag, func(t *testing.T) {
			configureTestTarget(t)
			t.Chdir(httpsAdoptionFixture(t, false))
			old, oldInput := applicationCoreBootstrap, appInitInput
			t.Cleanup(func() { applicationCoreBootstrap, appInitInput = old, oldInput })
			appInitInput = strings.NewReader("")
			called := false
			failure := errors.New("bootstrap blocked safely")
			applicationCoreBootstrap = func(ctx context.Context, in io.Reader, out io.Writer, opts runtimeUpOptions) (coreinstallation.State, error) {
				called = opts.Yes
				return coreinstallation.State{}, failure
			}
			err := runWithIO(context.Background(), []string{"app", "init", "--quick", flag}, io.Discard, io.Discard)
			if !called || !errors.Is(err, failure) {
				t.Fatalf("consent did not reach bootstrap: %v %v", called, err)
			}
			if _, err := os.Stat("baseharbor.yaml"); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("manifest written after failed bootstrap")
			}
		})
	}
}
