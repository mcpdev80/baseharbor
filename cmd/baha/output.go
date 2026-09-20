package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type cliOutputFormat string

const (
	outputHuman cliOutputFormat = "human"
	outputJSON  cliOutputFormat = "json"
)

func parseReadOutputArgs(args []string, command string) ([]string, cliOutputFormat, error) {
	format := outputHuman
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			format = outputJSON
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return nil, "", usageError(arg+" requires FORMAT", "Use -o json or --output json.")
			}
			i++
			value := strings.ToLower(strings.TrimSpace(args[i]))
			if value != "json" && value != "human" {
				return nil, "", usageError("unsupported output format "+value, "Use human or json.")
			}
			format = cliOutputFormat(value)
		case strings.HasPrefix(arg, "--output="):
			value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--output=")))
			if value != "json" && value != "human" {
				return nil, "", usageError("unsupported output format "+value, "Use human or json.")
			}
			format = cliOutputFormat(value)
		default:
			filtered = append(filtered, arg)
		}
	}
	if format == outputJSON && len(filtered) > 1 {
		return nil, "", usageError("baha "+command+" accepts at most one NAME", "Run it without NAME inside an application repository, or pass NAME explicitly.")
	}
	return filtered, format, nil
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode JSON output: %w", err)
	}
	return nil
}
