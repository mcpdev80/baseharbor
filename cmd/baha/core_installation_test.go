package main

import (
	"bufio"
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

func TestCoreRequiredNoninteractiveHasNoMutation(t *testing.T) {
	target := configureTestTarget(t)
	ctx := machineNoninteractiveContext(context.Background())
	err := requireApplicationCore(ctx, strings.NewReader(""), io.Discard)
	var failure *machine.Error
	if !errors.As(err, &failure) || failure.CauseCode != "core_required" || !failure.Retryable {
		t.Fatalf("bootstrap requirement: %v", err)
	}
	root, err := targetRuntimeStateRoot(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coreinstallation.Load(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("implicit mutation: %v", err)
	}
}

func TestCoreBootstrapPromptPreservesApplicationInput(t *testing.T) {
	configureTestTarget(t)
	previous := applicationCoreBootstrap
	t.Cleanup(func() { applicationCoreBootstrap = previous })
	calls := 0
	applicationCoreBootstrap = func(_ context.Context, _ io.Reader, _ io.Writer, options runtimeUpOptions) (coreinstallation.State, error) {
		calls++
		if options.MachineRole != coreinstallation.Deployment || !options.Yes || !options.ControlPlaneOnly {
			t.Fatalf("bootstrap defaults: %+v", options)
		}
		return coreinstallation.State{Ready: true}, nil
	}
	reader := bufio.NewReader(strings.NewReader("y\n2\napplication-answer\n"))
	var out bytes.Buffer
	if err := requireApplicationCore(context.Background(), reader, &out); err != nil {
		t.Fatal(err)
	}
	answer, err := reader.ReadString('\n')
	if err != nil || answer != "application-answer\n" || calls != 1 {
		t.Fatalf("workflow input lost: %q %v", answer, err)
	}
	if !strings.Contains(out.String(), "Set them up now?") || !strings.Contains(out.String(), "deployment machine") {
		t.Fatalf("missing setup questions: %s", out.String())
	}
}

func TestCoreBootstrapDeclineOrFailureDoesNotWriteApplication(t *testing.T) {
	for _, answer := range []string{"n\n", "y\n1\n"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			configureTestTarget(t)
			t.Chdir(httpsAdoptionFixture(t, false))
			previous, previousInput := applicationCoreBootstrap, appInitInput
			t.Cleanup(func() { applicationCoreBootstrap, appInitInput = previous, previousInput })
			calls := 0
			applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
				calls++
				return coreinstallation.State{}, errors.New("Core bootstrap failed")
			}
			appInitInput = strings.NewReader(answer)
			if err := runWithIO(context.Background(), []string{"app", "init", "--quick"}, io.Discard, io.Discard); err == nil {
				t.Fatal("application success after missing Core")
			}
			if _, err := os.Stat("baseharbor.yaml"); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("application written before Core readiness")
			}
			if (answer == "n\n" && calls != 0) || (answer != "n\n" && calls != 1) {
				t.Fatalf("bootstrap calls: %d", calls)
			}
		})
	}
}

func TestCoreFirstAppInitContinuesAfterSuccessfulBootstrap(t *testing.T) {
	configureTestTarget(t)
	t.Chdir(httpsAdoptionFixture(t, false))
	previous, previousInput := applicationCoreBootstrap, appInitInput
	t.Cleanup(func() { applicationCoreBootstrap, appInitInput = previous, previousInput })
	calls := 0
	applicationCoreBootstrap = func(context.Context, io.Reader, io.Writer, runtimeUpOptions) (coreinstallation.State, error) {
		calls++
		return coreinstallation.State{Ready: true}, nil
	}
	appInitInput = strings.NewReader("y\n1\n")
	if err := runWithIO(context.Background(), []string{"app", "init", "--quick"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("baseharbor.yaml"); err != nil || calls != 1 {
		t.Fatalf("original workflow did not continue: %v, calls=%d", err, calls)
	}
}
