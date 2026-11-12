package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestGuidedOTLPSignalSelection(t *testing.T) {
	old := appInitInput
	defer func() { appInitInput = old }()
	for _, tt := range []struct {
		name, input    string
		detected, want []string
	}{
		{name: "default traces", input: "\n", want: []string{"traces"}},
		{name: "detected default", input: "\n", detected: []string{"metrics", "logs"}, want: []string{"logs", "metrics"}},
		{name: "numbers", input: "1,2,3\n", want: []string{"logs", "metrics", "traces"}},
		{name: "named signals", input: "traces,logs\n", want: []string{"logs", "traces"}},
		{name: "duplicate selection", input: "1,traces\n", want: []string{"traces"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := strings.NewReader(tt.input)
			appInitInput = input
			got, err := promptOTLPSignals(bufio.NewReader(input), io.Discard, tt.detected)
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("signals = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestGuidedOTLPSignalsRetryAccidentalYesWithoutRestarting(t *testing.T) {
	old := appInitInput
	defer func() { appInitInput = old }()
	input := strings.NewReader("y\nunknown\n\nnext-answer\n")
	appInitInput = input
	reader := bufio.NewReader(input)
	var out bytes.Buffer
	got, err := promptOTLPSignals(reader, &out, nil)
	if err != nil || !reflect.DeepEqual(got, []string{"traces"}) {
		t.Fatalf("signals after correction = %v, %v", got, err)
	}
	if strings.Count(out.String(), "This is not a yes/no question.") != 2 {
		t.Fatalf("missing local correction guidance: %s", out.String())
	}
	if next, err := reader.ReadString('\n'); err != nil || next != "next-answer\n" {
		t.Fatalf("next wizard answer consumed: %q, %v", next, err)
	}
}

func TestGuidedOTLPSignalsEOFDoesNotAcceptInvalidSelection(t *testing.T) {
	old := appInitInput
	defer func() { appInitInput = old }()
	input := strings.NewReader("y\n")
	appInitInput = input
	got, err := promptOTLPSignals(bufio.NewReader(input), io.Discard, nil)
	if !errors.Is(err, io.EOF) || len(got) != 0 {
		t.Fatalf("invalid selection accepted on EOF: %v, %v", got, err)
	}
}
