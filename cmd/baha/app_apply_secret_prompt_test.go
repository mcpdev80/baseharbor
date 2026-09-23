package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/openbao"
)

type fakeApplicationSecretSetter struct {
	values map[string][]byte
}

func (f *fakeApplicationSecretSetter) Set(_ context.Context, _ string, key string, value []byte) error {
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	f.values[key] = append([]byte(nil), value...)
	return nil
}

func TestPromptAndStoreMissingRequiredSecretsInteractiveSuccess(t *testing.T) {
	oldInput := appApplySecretInput
	oldHidden := appApplySecretReadHidden
	oldTerminal := appApplySecretIsTerminal
	appApplySecretInput = strings.NewReader("\n")
	appApplySecretIsTerminal = func(io.Reader) bool { return true }
	appApplySecretReadHidden = func(_ io.Reader, _ io.Writer, key string) ([]byte, error) {
		return []byte("value-for-" + key), nil
	}
	t.Cleanup(func() {
		appApplySecretInput = oldInput
		appApplySecretReadHidden = oldHidden
		appApplySecretIsTerminal = oldTerminal
	})

	setter := &fakeApplicationSecretSetter{}
	var out bytes.Buffer
	err := promptAndStoreMissingRequiredSecrets(
		context.Background(),
		setter,
		"demo",
		[]openbao.RequiredSecretStatus{{Name: "API_TOKEN"}, {Name: "JWT_SECRET"}},
		&out,
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(setter.values["API_TOKEN"]) != "value-for-API_TOKEN" || string(setter.values["JWT_SECRET"]) != "value-for-JWT_SECRET" {
		t.Fatalf("stored values = %#v", setter.values)
	}
	if strings.Contains(out.String(), "value-for-") {
		t.Fatalf("secret value leaked to output: %s", out.String())
	}
	for _, want := range []string{"Missing required application secrets", "[OK] secret API_TOKEN stored securely", "[OK] secret JWT_SECRET stored securely"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestPromptAndStoreMissingRequiredSecretsCancelIsActionable(t *testing.T) {
	oldInput := appApplySecretInput
	oldTerminal := appApplySecretIsTerminal
	appApplySecretInput = strings.NewReader("n\n")
	appApplySecretIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() {
		appApplySecretInput = oldInput
		appApplySecretIsTerminal = oldTerminal
	})

	err := promptAndStoreMissingRequiredSecrets(
		context.Background(),
		&fakeApplicationSecretSetter{},
		"demo",
		[]openbao.RequiredSecretStatus{{Name: "API_TOKEN"}},
		&bytes.Buffer{},
	)
	if err == nil || !strings.Contains(err.Error(), "baha app secret set API_TOKEN") {
		t.Fatalf("expected actionable cancellation error, got %v", err)
	}
}

func TestPromptAndStoreMissingRequiredSecretsNonInteractiveFailsClosed(t *testing.T) {
	ctx := cli.WithOutputOptions(context.Background(), cli.OutputOptions{NonInteractive: true})
	err := promptAndStoreMissingRequiredSecrets(
		ctx,
		&fakeApplicationSecretSetter{},
		"demo",
		[]openbao.RequiredSecretStatus{{Name: "API_TOKEN"}},
		&bytes.Buffer{},
	)
	if err == nil || !strings.Contains(err.Error(), "baha app secret set API_TOKEN") || !strings.Contains(err.Error(), "--stdin") {
		t.Fatalf("expected deterministic remediation, got %v", err)
	}
}

func TestPromptAndStoreMissingRequiredSecretsEOF(t *testing.T) {
	oldInput := appApplySecretInput
	oldTerminal := appApplySecretIsTerminal
	appApplySecretInput = strings.NewReader("")
	appApplySecretIsTerminal = func(io.Reader) bool { return true }
	t.Cleanup(func() {
		appApplySecretInput = oldInput
		appApplySecretIsTerminal = oldTerminal
	})

	err := promptAndStoreMissingRequiredSecrets(
		context.Background(),
		&fakeApplicationSecretSetter{},
		"demo",
		[]openbao.RequiredSecretStatus{{Name: "API_TOKEN"}},
		&bytes.Buffer{},
	)
	if err == nil {
		t.Fatal("expected EOF to fail")
	}
}
