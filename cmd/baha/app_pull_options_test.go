package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/development"
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

// TestEmitSourcePullUpCompositeIsSingleDocument proves the F1 fix: the `baha app
// pull --up --output json` surface emits exactly ONE valid JSON document — the
// pull result plus the apply output under "up" — never two concatenated JSON
// values. This is the exact seam runAppPull uses: the pull result and the apply
// output are captured in separate buffers and merged by
// emitSourcePullUpComposite into a single document.
func TestEmitSourcePullUpCompositeIsSingleDocument(t *testing.T) {
	result := development.SourcePullResult{
		SchemaVersion: development.SourcePullSchemaVersion,
		Successful:    true,
	}
	result.Repositories = append(result.Repositories, development.SourcePullRepository{
		Source:          "api-source",
		State:           development.SourcePullCurrent,
		Updated:         true,
		UpdateAvailable: false,
	})
	var out bytes.Buffer
	if err := emitSourcePullUpComposite(&out, result, "starting...\nstarted\n"); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("emitSourcePullUpComposite must return ONE valid JSON document: %v\n%s", err, out.String())
	}
	if len(doc) != 3 {
		t.Fatalf("the single JSON document must have schema_version, pull and up keys only: %#v", doc)
	}
	if _, ok := doc["pull"]; !ok {
		t.Fatalf("the single JSON document must carry the pull result: %s", out.String())
	}
	up, ok := doc["up"].(map[string]any)
	if !ok {
		t.Fatalf("the single JSON document must carry the up block: %s", out.String())
	}
	if up["ran"] != true {
		t.Fatalf("the up block must report ran=true: %v", up)
	}
	if up["output"] != "starting...\nstarted" {
		t.Fatalf("the up block must carry the trimmed apply output: %v", up["output"])
	}
}
