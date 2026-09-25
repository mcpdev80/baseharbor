package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/term"
	"io"
	"os"
	"strconv"
	"strings"
)

func promptCapabilityList(reader *bufio.Reader, out io.Writer, defaults []bool, allowNone bool) ([]bool, error) {
	labels := []string{
		"SQL Database (PostgreSQL-compatible evidence)",
		"Cache (Redis/Valkey-compatible evidence)",
		"Object Storage (S3-compatible)",
		"Managed Secrets",
		"Metrics (/metrics)",
		"OTLP telemetry",
		"Application logs",
	}
	if input, ok := appInitInput.(interface{ Fd() uintptr }); ok && term.IsTerminal(input.Fd()) {
		return promptCapabilityTTY(reader, out, input.Fd(), labels, defaults, allowNone)
	}

	fmt.Fprintln(out, "\nSelect application capabilities (Enter keeps detected defaults; comma-separated numbers override):")
	for i, label := range labels {
		mark := " "
		if defaults[i] {
			mark = "x"
		}
		fmt.Fprintf(out, "[%s] %d. %s\n", mark, i+1, label)
	}
	line, err := readPrompt(reader, out, "> ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(line) == "" {
		selected := append([]bool(nil), defaults...)
		if !anySelected(selected) && !allowNone {
			return nil, errors.New("select at least one backend capability or configure an application workload")
		}
		return selected, nil
	}
	selected := make([]bool, len(labels))
	for _, raw := range strings.Split(line, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 1 || n > len(labels) {
			return nil, fmt.Errorf("invalid service selection %q", raw)
		}
		selected[n-1] = true
	}
	if !anySelected(selected) && !allowNone {
		return nil, errors.New("select at least one backend capability or configure an application workload")
	}
	return selected, nil
}

func promptCapabilityTTY(reader *bufio.Reader, out io.Writer, fd uintptr, labels []string, defaults []bool, allowNone bool) ([]bool, error) {
	selected := append([]bool(nil), defaults...)
	cursor := 0

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("enable interactive capability selection: %w", err)
	}
	defer term.Restore(fd, oldState)

	fmt.Fprintln(out, "\nSelect application capabilities")
	fmt.Fprintln(out, "Use ↑/↓ to move, Space to toggle, Enter to confirm.")
	fmt.Fprint(out, "\x1b[?25l")
	defer fmt.Fprint(out, "\x1b[?25h")

	renderCapabilityChoices(out, labels, selected, cursor, false)

	for {
		key, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		switch key {
		case 3:
			return nil, errors.New("capability selection cancelled")
		case '\r', '\n':
			if !anySelected(selected) && !allowNone {
				fmt.Fprint(out, "\a")
				continue
			}
			fmt.Fprint(out, "\r\n")
			return selected, nil
		case ' ':
			selected[cursor] = !selected[cursor]
			renderCapabilityChoices(out, labels, selected, cursor, true)
		case 'j':
			cursor = (cursor + 1) % len(labels)
			renderCapabilityChoices(out, labels, selected, cursor, true)
		case 'k':
			cursor = (cursor - 1 + len(labels)) % len(labels)
			renderCapabilityChoices(out, labels, selected, cursor, true)
		case 0x1b:
			next, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			if next != '[' {
				continue
			}
			direction, err := reader.ReadByte()
			if err != nil {
				return nil, err
			}
			switch direction {
			case 'A':
				cursor = (cursor - 1 + len(labels)) % len(labels)
				renderCapabilityChoices(out, labels, selected, cursor, true)
			case 'B':
				cursor = (cursor + 1) % len(labels)
				renderCapabilityChoices(out, labels, selected, cursor, true)
			}
		}
	}
}

func renderCapabilityChoices(out io.Writer, labels []string, selected []bool, cursor int, redraw bool) {
	if redraw {
		fmt.Fprintf(out, "\x1b[%dA\r\x1b[J", len(labels))
	}
	for i, label := range labels {
		pointer := " "
		if i == cursor {
			pointer = ">"
		}
		mark := " "
		if selected[i] {
			mark = "x"
		}
		fmt.Fprintf(out, "%s [%s] %d. %s\r\n", pointer, mark, i+1, label)
	}
}

func anySelected(values []bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}

func promptMetricsTarget(reader *bufio.Reader, out io.Writer, workloadServices []string) (string, int, error) {
	if len(workloadServices) == 0 {
		return "", 0, errors.New("metrics collection requires an application workload service")
	}
	defaultService := workloadServices[0]
	service, err := promptLine(reader, out, "Metrics workload service", defaultService)
	if err != nil {
		return "", 0, err
	}
	found := false
	for _, candidate := range workloadServices {
		if candidate == service {
			found = true
			break
		}
	}
	if !found {
		return "", 0, fmt.Errorf("metrics service %q is not a selected workload service", service)
	}
	value, err := promptLine(reader, out, "Metrics container port", "")
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("invalid metrics container port %q", value)
	}
	return service, port, nil
}

func promptAmbiguousComposeServices(reader *bufio.Reader, out io.Writer, services []string) ([]string, error) {
	services = uniqueSorted(services)
	if len(services) == 0 {
		return nil, nil
	}
	fmt.Fprintln(out, "\nThese Compose services look infrastructure-like but could not be classified safely.")
	fmt.Fprintln(out, "Select any that are application workload services (Enter = none; remaining services stay replaceable infrastructure):")
	for i, service := range services {
		fmt.Fprintf(out, "  %d. %s\n", i+1, service)
	}
	line, err := readPrompt(reader, out, "> ")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(line) == "" {
		return nil, nil
	}
	var selected []string
	for _, raw := range strings.Split(line, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 1 || n > len(services) {
			return nil, fmt.Errorf("invalid ambiguous service selection %q", raw)
		}
		selected = append(selected, services[n-1])
	}
	return uniqueSorted(selected), nil
}

func promptServiceInstances(reader *bufio.Reader, out io.Writer, label string, detected []string) ([]string, error) {
	detected = uniqueSorted(detected)
	defaultValue := "default"
	if len(detected) > 1 {
		defaultValue = strings.Join(detected, ",")
	}
	line, err := promptLine(reader, out, label+" instances (comma-separated)", defaultValue)
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" || line == "default" {
		return nil, nil
	}
	var names []string
	for _, raw := range strings.Split(line, ",") {
		name := slugifyAppName(raw)
		if name == "" {
			return nil, fmt.Errorf("invalid %s instance name %q", label, raw)
		}
		names = append(names, name)
	}
	return uniqueSorted(names), nil
}

func promptSecretPolicies(reader *bufio.Reader, out io.Writer, candidates []string, sources map[string]string) ([]guidedSecretPolicy, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	fmt.Fprintln(out, "\nApplication secrets")
	fmt.Fprintln(out, "Detected names are evidence only. Repository values are never displayed or imported.")
	var policies []guidedSecretPolicy
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		source := sources[candidate]
		fmt.Fprintf(out, "\nPotential secret detected\n  Source: %s\n  Detected variable: %s\n", source, candidate)
		use, err := promptYesNo(reader, out, "Manage this application secret with BaseHarbor?", true)
		if err != nil {
			return nil, err
		}
		if !use {
			continue
		}
		name, err := promptLine(reader, out, "BaseHarbor secret name", candidate)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("application secret name cannot be empty")
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate application secret %q", name)
		}
		seen[name] = struct{}{}
		required, err := promptYesNo(reader, out, "Required for application startup?", true)
		if err != nil {
			return nil, err
		}
		provision, err := promptSecretProvisionPolicy(reader, out)
		if err != nil {
			return nil, err
		}
		policies = append(policies, guidedSecretPolicy{
			Name: name, Source: source, Required: required, Provision: provision,
		})
	}
	return policies, nil
}

func promptSecretProvisionPolicy(reader *bufio.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "How should it be provided?")
	fmt.Fprintln(out, "  1. Ask for value during first apply")
	fmt.Fprintln(out, "  2. Generate automatically")
	fmt.Fprintln(out, "  3. Configure later")
	value, err := promptLine(reader, out, "Selection", "1")
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(value) {
	case "1":
		return "prompt", nil
	case "2":
		return "generate", nil
	case "3":
		return "later", nil
	default:
		return "", fmt.Errorf("invalid secret provision selection %q", value)
	}
}

func promptCompose(reader *bufio.Reader, out io.Writer, candidates []string) (string, error) {
	fmt.Fprintln(out, "\nMultiple Compose files were detected. Select the application workload:")
	for i, candidate := range candidates {
		fmt.Fprintf(out, "  %d. %s\n", i+1, candidate)
	}
	line, err := readPrompt(reader, out, "> ")
	if err != nil {
		return "", err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(candidates) {
		return "", fmt.Errorf("invalid Compose selection %q", line)
	}
	return candidates[n-1], nil
}

func promptLine(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	prompt := label
	if defaultValue != "" {
		prompt += " [" + defaultValue + "]"
	}
	line, err := readPrompt(reader, out, prompt+": ")
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func promptYesNo(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}
	line, err := readPrompt(reader, out, label+" "+suffix+" ")
	if err != nil {
		return false, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return defaultYes, nil
	}
	switch line {
	case "y", "yes", "j", "ja":
		return true, nil
	case "n", "no", "nein":
		return false, nil
	default:
		return false, fmt.Errorf("expected yes or no, got %q", line)
	}
}

func readPrompt(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if errors.Is(err, io.EOF) && line == "" {
		return "", io.EOF
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func appInitReaderIsTerminal(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return true
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
