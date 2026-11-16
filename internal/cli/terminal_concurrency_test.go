package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestTerminalActivityAndResultsSerializeSharedWriter(t *testing.T) {
	var out bytes.Buffer
	term := NewTerminal(WithOutputOptions(t.Context(), OutputOptions{Verbose: true}), &out, &out)
	err := term.Activity(context.Background(), "concurrent rendering", func(w io.Writer) error {
		var workers sync.WaitGroup
		for i := 0; i < 4; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for j := 0; j < 100; j++ {
					term.Header("app", "dev")
					term.Section("routes")
					term.Result("READY", "route", "verified")
					term.activityLine("INFO", "progress")
					term.Diagnostic("diagnostic\n")
				}
			}()
		}
		workers.Wait()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "\nroutes\n") != 400 || strings.Count(out.String(), "[INFO] progress\n") != 400 {
		t.Fatal("concurrent output lost or corrupted a result")
	}
}
