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
