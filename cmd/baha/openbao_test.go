package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestOpenBaoRecoveryArgumentParsing(t *testing.T) {
	path, err := parseRecoveryFileArg("bootstrap", []string{"--recovery-file", "/secure/recovery.json"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/secure/recovery.json" {
		t.Fatalf("unexpected recovery path %q", path)
	}
	path, err = parseRecoveryFileArg("unseal", []string{"--recovery-file=/secure/recovery.json"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/secure/recovery.json" {
		t.Fatalf("unexpected recovery path %q", path)
	}
}

func TestOpenBaoRecoveryArgumentRequired(t *testing.T) {
	if _, err := parseRecoveryFileArg("bootstrap", nil); err == nil {
		t.Fatal("expected missing recovery file to fail")
	}
	if _, err := parseRecoveryFileArg("bootstrap", []string{"--unknown"}); err == nil {
		t.Fatal("expected unknown option to fail")
	}
}

func TestOpenBaoHelpIsDiscoverable(t *testing.T) {
	for _, args := range [][]string{{"openbao", "--help"}, {"openbao", "bootstrap", "--help"}, {"openbao", "unseal", "--help"}} {
		var out bytes.Buffer
		if err := runWithIO(context.Background(), args, &out, &out); err != nil {
			t.Fatalf("help %v failed: %v", args, err)
		}
		if !strings.Contains(out.String(), "Usage:") {
			t.Fatalf("help %v missing usage: %s", args, out.String())
		}
	}
}
