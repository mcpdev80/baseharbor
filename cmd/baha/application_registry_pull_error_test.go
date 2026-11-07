package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type ociPullErrorProvider struct {
	bhruntime.RuntimeProvider
	err   error
	pulls []string
}

func (p *ociPullErrorProvider) PullImage(_ context.Context, image string) error {
	p.pulls = append(p.pulls, image)
	return p.err
}

func TestApplicationRegistrySourcePullErrorClassification(t *testing.T) {
	const resolved = "registry.example.invalid/app@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	original := resolveRegistrySource
	resolveRegistrySource = func(context.Context, string, string) (string, error) {
		return resolved, nil
	}
	t.Cleanup(func() { resolveRegistrySource = original })

	const secret = "very-sensitive-registry-token"
	tests := []struct {
		name       string
		pullErr    error
		missing    bool
	}{
		{
			name:    "native oras executable missing",
			pullErr: fmt.Errorf("native pull: %w", &exec.Error{Name: "oras", Err: exec.ErrNotFound}),
			missing: true,
		},
		{
			name:    "registry failure with sensitive output",
			pullErr: errors.New("registry authorization failed: " + secret),
		},
		{
			name:    "native tool exits unsuccessfully",
			pullErr: errors.New("oras: exited with status 7: " + secret),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &ociPullErrorProvider{err: tc.pullErr}
			err := pullResolvedRuntimeImage(context.Background(), provider, "registry.example.invalid/app:edge", "edge")
			if err == nil {
				t.Fatal("failed pull returned nil")
			}
			if got := errors.Is(err, exec.ErrNotFound); got != tc.missing {
				t.Fatalf("exec.ErrNotFound identity=%t, want %t: %v", got, tc.missing, err)
			}
			if got := errors.Is(err, ErrRegistryNativeToolMissing); got != tc.missing {
				t.Fatalf("missing-oras classification=%t, want %t: %v", got, tc.missing, err)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("sensitive command output leaked through pull error")
			}
			if len(provider.pulls) != 1 || provider.pulls[0] != resolved {
				t.Fatalf("pull did not target the resolved immutable image: %v", provider.pulls)
			}
		})
	}
}
