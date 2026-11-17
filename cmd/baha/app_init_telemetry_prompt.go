package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
)

func promptOTLPSignals(reader *bufio.Reader, out io.Writer, detected []string) ([]string, error) {
	signals := []string{"traces", "metrics", "logs"}
	labels := []string{"Traces (request tracing)", "Metrics (OTLP measurements)", "Logs (OTLP log records)"}
	defaults := make([]bool, len(signals))
	for i, signal := range signals {
		for _, value := range detected {
			defaults[i] = defaults[i] || value == signal
		}
	}
	if !anySelected(defaults) {
		defaults[0] = true
	}
	selectedSignals := func(selected []bool) []string {
		var result []string
		for i, enabled := range selected {
			if enabled {
				result = append(result, signals[i])
			}
		}
		return uniqueSorted(result)
	}
	if input, ok := appInitInput.(interface{ Fd() uintptr }); ok && term.IsTerminal(input.Fd()) {
		selected, err := promptCheckboxTTY(reader, out, input.Fd(), "Select OTLP signals", labels, defaults, false)
		if err != nil {
			return nil, err
		}
		return selectedSignals(selected), nil
	}
	fmt.Fprintln(out, "\nSelect OTLP signals (comma-separated numbers or signal names):")
	var defaultNumbers []string
	for i, label := range labels {
		mark := " "
		if defaults[i] {
			mark = "x"
			defaultNumbers = append(defaultNumbers, strconv.Itoa(i+1))
		}
		fmt.Fprintf(out, "  [%s] %d. %s\n", mark, i+1, label)
	}
	for {
		value, err := promptLine(reader, out, "Signals (Enter keeps the selected defaults)", strings.Join(defaultNumbers, ","))
		if err != nil {
			return nil, err
		}
		selected := make([]bool, len(signals))
		valid := true
		for _, part := range strings.Split(value, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			index := -1
			for i, signal := range signals {
				if part == signal || part == strconv.Itoa(i+1) {
					index = i
					break
				}
			}
			if index < 0 {
				valid = false
				break
			}
			selected[index] = true
		}
		if valid && anySelected(selected) {
			return selectedSignals(selected), nil
		}
		fmt.Fprintln(out, "Choose 1 (traces), 2 (metrics), 3 (logs), or a combination such as 1,2. This is not a yes/no question.")
	}
}
