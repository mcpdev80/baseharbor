package main

import (
 "bytes"
 "context"
 "errors"
 "strings"
 "testing"

 "github.com/mcpdev80/baseharbor/internal/cli"
)

func TestAppDownRejectsUnknownOptionsBeforeRuntime(t *testing.T) {
 for _, args := range [][]string{{"app", "down", "--yes"}, {"app", "down", "--unknown"}} {
  var out bytes.Buffer
  err := runWithIO(context.Background(), args, &out, &out)
  if err == nil { t.Fatalf("%v should fail", args) }
  var usage *cli.UsageError
  if !errors.As(err, &usage) || !strings.Contains(usage.Message, "unknown option") {
   t.Fatalf("%v: want typed unknown option, got %v", args, err)
  }
 }
}

func TestHumanNewExplicitObjectKinds(t *testing.T) {
 for _, tc := range []struct{ kind string; next string }{
  {"target", "target creation needs"},
  {"provider", "provider ID is required"},
  {"bogus", "unsupported creation type"},
 } {
  var out bytes.Buffer
  err := runWithIO(context.Background(), []string{"new", tc.kind}, &out, &out)
  if err == nil || !strings.Contains(err.Error(), tc.next) {
   t.Fatalf("new %s: want %q, got %v", tc.kind, tc.next, err)
  }
 }
}
