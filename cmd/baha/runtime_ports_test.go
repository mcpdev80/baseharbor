package main

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestParseRuntimeUpOptions(t *testing.T) {
	opts, err := parseRuntimeUpOptions([]string{"--yes", "--postgres-port", "15432", "--openbao-port", "18200"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Yes || opts.PostgresPort != 15432 || opts.OpenBaoPort != 18200 {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestParseRuntimeUpOptionsRejectsInvalidPort(t *testing.T) {
	if _, err := parseRuntimeUpOptions([]string{"--postgres-port", "70000"}); err == nil {
		t.Fatal("expected invalid port error")
	}
}

func TestPortAvailableDetectsOccupiedLoopbackPort(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if portAvailable(port) {
		t.Fatalf("port %d reported available while listener is active", port)
	}
}

func TestReadPortChoiceAcceptsDefaultAndOverride(t *testing.T) {
	choice, err := readPortChoice(bufioReader("\n"), 15432)
	if err != nil || choice != 15432 {
		t.Fatalf("default choice = %d, err=%v", choice, err)
	}
	choice, err = readPortChoice(bufioReader("25432\n"), 15432)
	if err != nil || choice != 25432 {
		t.Fatalf("override choice = %d, err=%v", choice, err)
	}
}

func TestRuntimeUpGuidedRejectsSameExplicitPorts(t *testing.T) {
	var out bytes.Buffer
	err := runtimeUpGuided(context.Background(), strings.NewReader(""), &out, runtimeUpOptions{Yes: true, PostgresPort: 15432, OpenBaoPort: 15432})
	if err == nil || !strings.Contains(err.Error(), "same host port") {
		t.Fatalf("error = %v, want same host port rejection", err)
	}
}

func TestRuntimePortDefaultsRemainStable(t *testing.T) {
	if bhruntime.DefaultPostgresPort != 5432 || bhruntime.DefaultOpenBaoPort != 8200 {
		t.Fatalf("unexpected defaults: postgres=%d openbao=%d", bhruntime.DefaultPostgresPort, bhruntime.DefaultOpenBaoPort)
	}
}

func bufioReader(value string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(value))
}
