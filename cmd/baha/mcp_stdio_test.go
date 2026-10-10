package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const eofInitialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"eof-test","version":"1"}}}` + "\n"

type mcpTestWriter struct{ io.Writer }

func (mcpTestWriter) Close() error { return nil }

func TestMCPStdioEOFFlushesDecodedRequests(t *testing.T) {
	for _, input := range []string{eofInitialize, eofInitialize + `{"jsonrpc":"2.0","id":2,"method":"missing-method"}` + "\n", ""} {
		var out bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		transport := &drainingMCPTransport{inner: &mcp.IOTransport{Reader: io.NopCloser(strings.NewReader(input)), Writer: mcpTestWriter{&out}}, timeout: time.Second}
		err := newMCPServer(application.DefaultStore()).Run(ctx, transport)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Count(input, "\n")
		dec := json.NewDecoder(&out)
		for i := 0; i < want; i++ {
			var reply map[string]any
			if err := dec.Decode(&reply); err != nil {
				t.Fatalf("lost decoded request: %v", err)
			}
			if reply["id"] == float64(1) && reply["result"] == nil {
				t.Fatalf("initialize response: %v", reply)
			}
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			t.Fatalf("extra protocol output: %v %v", extra, err)
		}
	}
}

func TestMCPStdioPipeSubprocessEOF(t *testing.T) {
	if os.Getenv("BASEHARBOR_TEST_MCP_EOF_HELPER") == "1" {
		if err := runMCPServer(context.Background(), application.DefaultStore()); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	configureTestTarget(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMCPStdioPipeSubprocessEOF$")
	cmd.Env = append(os.Environ(), "BASEHARBOR_TEST_MCP_EOF_HELPER=1")
	cmd.Stdin = strings.NewReader(eofInitialize)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(out, &reply); err != nil || reply.ID != 1 || len(reply.Result) == 0 {
		t.Fatalf("closed pipe reply: %s %v", out, err)
	}
}

type eofMCPConnection struct{}

func (eofMCPConnection) Read(context.Context) (jsonrpc.Message, error) { return nil, io.EOF }
func (eofMCPConnection) Write(context.Context, jsonrpc.Message) error  { return nil }
func (eofMCPConnection) Close() error                                  { return nil }
func (eofMCPConnection) SessionID() string                             { return "" }

func TestMCPStdioEOFDrainIsBoundedAndCloseUnblocks(t *testing.T) {
	id, _ := jsonrpc.MakeID(float64(1))
	for _, closeFirst := range []bool{false, true} {
		c := &drainingMCPConnection{Connection: eofMCPConnection{}, timeout: 20 * time.Millisecond, pending: map[jsonrpc.ID]int{id: 1}, changed: make(chan struct{}), closed: make(chan struct{})}
		if closeFirst {
			c.Close()
		}
		_, err := c.Read(context.Background())
		if err == nil || err == io.EOF {
			t.Fatalf("silently lost reply: %v", err)
		}
	}
}

func TestMCPStdioOpenInputRetainsNormalClientSession(t *testing.T) {
	configureTestTarget(t)
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- newMCPServer(application.DefaultStore()).Run(ctx, &drainingMCPTransport{inner: &mcp.IOTransport{Reader: inReader, Writer: outWriter}, timeout: time.Second})
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "normal-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: outReader, Writer: inWriter}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.ListTools(ctx, nil)
	if err != nil || len(result.Tools) == 0 {
		t.Fatalf("open stdin discovery: %v %v", result, err)
	}
	session.Close()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("stdio session did not close")
	}
}
