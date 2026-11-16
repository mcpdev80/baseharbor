package main

import (
	"context"
	"io"
	"testing"
)

func setMaintenanceLifecycleOwner(t *testing.T, fn func(ctx context.Context, operation string, resolved resolvedApplication, args []string, out, errOut io.Writer) error) {
	t.Helper()
	previous := maintenanceLifecycleOwner
	maintenanceLifecycleOwner = fn
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
}
