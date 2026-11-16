package traces

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/objectstorage"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

type rejectedTempoPreparationRuntime struct {
	bhruntime.RuntimeProvider
	configured, started int
}

func (r *rejectedTempoPreparationRuntime) ConfigProject(context.Context, string, string, string) error {
	r.configured++
	return errors.New("unexpected runtime after failed preparation")
}
func (r *rejectedTempoPreparationRuntime) UpProject(context.Context, string, string, string) error {
	r.started++
	return nil
}

func TestTempoHAPreparationErrorCannotReachRuntime(t *testing.T) {
	runtime := &rejectedTempoPreparationRuntime{}
	yes := true
	m := application.WithAvailabilityOverride(application.WithHA(application.New("preparation-error", "test", false, false, false), true), "traces", &yes, 3)
	storageCalls := 0
	prepare := func(context.Context, objectstorage.Runtime, serviceaccess.Issuer, string, string, application.Manifest) (objectstorage.PlatformBucket, error) {
		storageCalls++
		return objectstorage.PlatformBucket{Network: "prepared-storage"}, nil
	}
	_, err := provisionTempoAt(t.Context(), runtime, nil, m, t.TempDir(), "isolated", prepare)
	if err == nil || !strings.Contains(err.Error(), "instance override") {
		t.Fatalf("original file preparation error was lost: %v", err)
	}
	if storageCalls != 1 || runtime.configured != 0 || runtime.started != 0 {
		t.Fatal("failed HA preparation reached runtime configuration or execution")
	}
}
