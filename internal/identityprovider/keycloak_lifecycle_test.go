package identityprovider

import (
	"context"
	"testing"
)

type recordingKeycloakRuntime struct {
	configured []string
	started    []string
	destroyed  []string
}

func (r *recordingKeycloakRuntime) ConfigProject(_ context.Context, project, composeFile, envFile string) error {
	r.configured = []string{project, composeFile, envFile}
	return nil
}

func (r *recordingKeycloakRuntime) UpProject(_ context.Context, project, composeFile, envFile string) error {
	r.started = []string{project, composeFile, envFile}
	return nil
}

func (r *recordingKeycloakRuntime) DestroyProject(_ context.Context, project, composeFile, envFile string) error {
	r.destroyed = []string{project, composeFile, envFile}
	return nil
}

func TestKeycloakLifecycleHidesComposeVocabularyFromDriver(t *testing.T) {
	runtime := &recordingKeycloakRuntime{}
	lifecycle := NewKeycloakLifecycle(runtime)
	files := KeycloakFiles{Project: "identity-provider", Compose: "/state/provider.yaml", Env: "/state/provider.env"}

	if err := lifecycle.Validate(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Apply(context.Background(), files); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Destroy(context.Background(), files); err != nil {
		t.Fatal(err)
	}

	for name, got := range map[string][]string{
		"validate": runtime.configured,
		"apply":    runtime.started,
		"destroy":  runtime.destroyed,
	} {
		if len(got) != 3 || got[0] != files.Project || got[1] != files.Compose || got[2] != files.Env {
			t.Fatalf("%s mapping = %#v", name, got)
		}
	}
}
