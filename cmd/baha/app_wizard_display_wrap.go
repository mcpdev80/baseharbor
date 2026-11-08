package main

import (
	"strings"
	"unicode/utf8"
)

// wrapWizardDisplay wraps read-only provider descriptions without changing
// the selected provider or the controller's immutable decision state.
func wrapWizardDisplay(message string, width int) string {
	if width < 12 {
		return message
	}
	lines := strings.Split(strings.TrimSuffix(message, "\n"), "\n")
	var output []string
	for _, line := range lines {
		if utf8.RuneCountInString(line) <= width {
			output = append(output, line)
			continue
		}
		indent := "  "
		current := ""
		for _, word := range strings.Fields(line) {
			if current == "" {
				current = word
				continue
			}
			if utf8.RuneCountInString(current)+1+utf8.RuneCountInString(word) > width {
				output = append(output, current)
				current = indent + word
			} else {
				current += " " + word
			}
		}
		if current != "" {
			output = append(output, current)
		}
	}
	return strings.Join(output, "\n")
}
