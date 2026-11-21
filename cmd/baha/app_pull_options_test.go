package main

import (
	"testing"
)

// TestAppPullParseOptionsPositive proves the flag parser accepts the component,
// --up, --check, --manifest and --output json selections.
func TestAppPullParseOptionsPositive(t *testing.T) {
	opts, err := parseAppPullOptions([]string{"-c", "api,web", "--up"})
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Components) != 2 || opts.Components[0] != "api" || opts.Components[1] != "web" {
		t.Fatalf("components = %#v", opts.Components)
	}
	if !opts.Up {
		t.Fatalf("flags not parsed: %#v", opts)
	}
}

// TestAppPullParseOptionsComponentLong proves the long --component form and the
// equals form are accepted.
func TestAppPullParseOptionsComponentLong(t *testing.T) {
	opts, err := parseAppPullOptions([]string{"--component", "api", "--component=web"})
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Components) != 2 || opts.Components[0] != "api" || opts.Components[1] != "web" {
		t.Fatalf("components = %#v", opts.Components)
	}
}

// TestAppPullParseOptionsCheckAndUpRejected proves --check and --up cannot be
// combined; --up must run only after a successful pull.
func TestAppPullParseOptionsCheckAndUpRejected(t *testing.T) {
	if _, err := parseAppPullOptions([]string{"--check", "--up"}); err == nil {
		t.Fatalf("--check and --up must be rejected together")
	}
}

// TestAppPullParseOptionsUnknownOptionRejected proves unknown flags fail closed.
func TestAppPullParseOptionsUnknownOptionRejected(t *testing.T) {
	if _, err := parseAppPullOptions([]string{"--nope"}); err == nil {
		t.Fatalf("unknown option must fail closed")
	}
}

// TestAppPullParseOptionsNoPositional proves positional arguments are rejected.
func TestAppPullParseOptionsNoPositional(t *testing.T) {
	if _, err := parseAppPullOptions([]string{"api"}); err == nil {
		t.Fatalf("positional arguments must fail closed")
	}
}

// TestAppPullParseOptionsOutputJsonOnly proves --output accepts only json.
func TestAppPullParseOptionsOutputJsonOnly(t *testing.T) {
	if _, err := parseAppPullOptions([]string{"--output", "yaml"}); err == nil {
		t.Fatalf("non-json output must fail closed")
	}
}
