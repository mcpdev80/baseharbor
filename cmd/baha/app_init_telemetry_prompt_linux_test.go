//go:build linux

package main

import (
	"bufio"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
)

func TestGuidedOTLPSignalCheckboxTerminal(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	old := appInitInput
	appInitInput = slave
	defer func() { appInitInput = old }()
	before, err := term.GetState(slave.Fd())
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, keys string
		want       []string
		cancel     bool
	}{
		{name: "accidental yes keeps traces", keys: "y\r", want: []string{"traces"}},
		{name: "arrow and space add metrics", keys: "\x1b[B \r", want: []string{"metrics", "traces"}},
		{name: "cannot confirm empty selection", keys: " \r\x1b[B \r", want: []string{"metrics"}},
		{name: "cancel", keys: "\x03", cancel: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := promptOTLPSignals(bufio.NewReader(strings.NewReader(tt.keys)), io.Discard, nil)
			if tt.cancel {
				if err == nil || len(got) != 0 {
					t.Fatalf("cancel accepted selection: %v, %v", got, err)
				}
			} else if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("signals = %v, %v; want %v", got, err, tt.want)
			}
			after, err := term.GetState(slave.Fd())
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("terminal state not restored: %v", err)
			}
		})
	}
}
