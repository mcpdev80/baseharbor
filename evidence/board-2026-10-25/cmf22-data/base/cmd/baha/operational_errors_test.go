package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

func TestClassifyOperationalFailureNormalizesDockerAndPodmanPortConflicts(t *testing.T) {
	cases := []error{
		errors.New("Error response from daemon: Bind for ::1:8080 failed: port is already allocated"),
		errors.New("podman: failed to listen on host port 8080: address already in use"),
	}
	for _, input := range cases {
		err := classifyOperationalFailure(input, "api")
		var typed *machine.Error
		if !errors.As(err, &typed) {
			t.Fatalf("expected typed error for %q, got %T", input, err)
		}
		if typed.Code != machine.ErrorPortConflict {
			t.Fatalf("code = %q for %q", typed.Code, input)
		}
		if typed.Resource != "api" {
			t.Fatalf("resource = %q", typed.Resource)
		}
		if strings.Contains(typed.Message, "daemon") || strings.Contains(typed.Message, "podman") {
			t.Fatalf("human message leaked runtime noise: %q", typed.Message)
		}
	}
}

func TestClassifyOperationalFailureMapsImagePullAndRuntimeUnavailable(t *testing.T) {
	tests := []struct {
		err  error
		code machine.ErrorCode
	}{
		{errors.New("pull access denied for private/image"), machine.ErrorImagePullFailed},
		{errors.New("Cannot connect to the Docker daemon"), machine.ErrorRuntimeUnavailable},
		{errors.New("unauthorized: authentication required"), machine.ErrorAuthenticationFailed},
		{errors.New("compose config invalid yaml"), machine.ErrorInvalidWorkload},
	}
	for _, tc := range tests {
		var typed *machine.Error
		if !errors.As(classifyOperationalFailure(tc.err, "api"), &typed) {
			t.Fatalf("expected typed error for %v", tc.err)
		}
		if typed.Code != tc.code {
			t.Fatalf("%v => %q, want %q", tc.err, typed.Code, tc.code)
		}
	}
}

func TestFormatCLIErrorRendersTypedOperationalErrorConcise(t *testing.T) {
	err := &machine.Error{
		Code:        machine.ErrorPortConflict,
		Message:     "Port 8080 is already in use.",
		Resource:    "demo-app",
		Remediation: "requires developer input",
		Next:        "Free port 8080 or make the Compose binding configurable.",
		Cause:       errors.New("docker raw runtime noise should stay diagnostic"),
	}
	var out bytes.Buffer
	formatCLIError(&out, err)
	text := out.String()
	for _, want := range []string{
		"Port 8080 is already in use.",
		"Affected",
		"demo-app",
		"Resolution",
		"requires developer input",
		"What to do",
		"Free port 8080",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "docker raw runtime noise") {
		t.Fatalf("raw runtime detail leaked into primary error:\n%s", text)
	}
}

func TestMachineOperationalErrorPreservesStructuredFields(t *testing.T) {
	input := &machine.Error{
		Code:        machine.ErrorPortConflict,
		Message:     "Port 8080 is already in use.",
		Resource:    "api",
		Remediation: "requires developer input",
		Next:        "Choose a free port.",
	}
	got := machine.Classify(input)
	if got.Code != input.Code || got.Resource != input.Resource || got.Remediation != input.Remediation || got.Next != input.Next {
		t.Fatalf("structured error changed: %#v", got)
	}
}
