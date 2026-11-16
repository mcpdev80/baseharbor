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
	for _, input := range []string{eofInitialize, strings.TrimSuffix(eofInitialize, "\n"), eofInitialize + `{"jsonrpc":"2.0","id":2,"method":"missing-method"}` + "\n", ""} {
		var out bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		transport := &drainingMCPTransport{reader: io.NopCloser(strings.NewReader(input)), writer: mcpTestWriter{&out}, timeout: time.Second}
		err := newMCPServer(application.DefaultStore()).Run(ctx, transport)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		want := strings.Count(input, "\n")
		if input != "" && !strings.HasSuffix(input, "\n") {
			want++
		}
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

func TestMCPStdioEOFDrainIsBoundedAndCloseUnblocks(t *testing.T) {
	id, _ := jsonrpc.MakeID(float64(1))
	for _, closeFirst := range []bool{false, true} {
		d := &mcpEOFDrain{ctx: context.Background(), timeout: 20 * time.Millisecond, pending: map[jsonrpc.ID]int{id: 1}, changed: make(chan struct{}), closed: make(chan struct{})}
		if closeFirst {
			d.close()
		}
		err := d.wait()
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
		done <- newMCPServer(application.DefaultStore()).Run(ctx, &drainingMCPTransport{reader: inReader, writer: outWriter, timeout: time.Second})
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

func TestMCPStdioPreservesSDKNegotiatedBatchRejection(t *testing.T) {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- newMCPServer(application.DefaultStore()).Run(ctx, &drainingMCPTransport{reader: inReader, writer: outWriter, timeout: time.Second})
	}()
	if _, err := io.WriteString(inWriter, eofInitialize); err != nil {
		t.Fatal(err)
	}
	var initialized map[string]any
	if err := json.NewDecoder(outReader).Decode(&initialized); err != nil || initialized["result"] == nil {
		t.Fatalf("initialize: %v %v", initialized, err)
	}
	_, err := io.WriteString(inWriter, `[{"jsonrpc":"2.0","id":2,"method":"ping"},{"jsonrpc":"2.0","id":3,"method":"ping"}]`+"\n")
	if err != nil {
		t.Fatal(err)
	}
	inWriter.Close()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "batching is not supported") {
			t.Fatalf("negotiated SDK policy lost: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("batch rejection did not close the session")
	}
}
