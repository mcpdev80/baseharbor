package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/preflight"
)

type tuiDoctorResult struct {
	Application string                     `json:"application"`
	Environment string                     `json:"environment"`
	Healthy     bool                       `json:"healthy"`
	Checks      []preflight.Result         `json:"checks"`
	TLS         *applicationTLSObservation `json:"tls,omitempty"`
}

type tuiApplicationStatusResult struct {
	application.StatusResult
	TLS *applicationTLSObservation `json:"tls,omitempty"`
}

type tuiStatusMsg struct {
	result application.StatusResult
	tls    *applicationTLSObservation
	doctor tuiDoctorResult
	err    error
}

type tuiModel struct {
	ctx           context.Context
	store         application.Store
	result        application.StatusResult
	tls           *applicationTLSObservation
	doctor        tuiDoctorResult
	err           error
	loading       bool
	tab           int
	width         int
	height        int
	reducedMotion bool
	noColor       bool
}

func tuiCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "tui",
		Summary: "Open the interactive BaseHarbor status dashboard",
		Usage:   "baha tui",
		Long:    "Opens a read-only terminal dashboard backed by the same application status model as 'baha status'. Use Tab or left/right to switch views, r to refresh and q or Ctrl-C to quit.",
		Examples: []string{
			"baha tui",
			"BASEHARBOR_REDUCED_MOTION=1 baha tui",
		},
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 0 {
				return usageError("baha tui does not accept arguments", "Run 'baha tui --help' for usage.")
			}
			opts := cli.OutputOptionsFromContext(ctx)
			if opts.Plain {
				return usageError("TUI is unavailable in --plain mode", "Use 'baha status --plain' or 'baha doctor --plain' instead.")
			}
			if opts.NonInteractive {
				return usageError("TUI is unavailable in --no-input mode", "Use 'baha status -o json' for automation.")
			}
			if !cli.IsTerminal(out) || !readerIsTerminal(os.Stdin) {
				return usageError("TUI requires an interactive terminal", "Use 'baha status' or 'baha status -o json' when piping or running in CI.")
			}
			if !inApplicationRepository() {
				return usageError("TUI currently requires an application repository", "Run inside a repository containing baseharbor.yaml.")
			}

			model := tuiModel{
				ctx:           ctx,
				store:         store,
				loading:       true,
				reducedMotion: opts.ReducedMotion,
				noColor:       opts.NoColor || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb",
			}
			program := tea.NewProgram(model)
			if _, err := program.Run(); err != nil {
				return fmt.Errorf("run BaseHarbor TUI: %w", err)
			}
			return nil
		},
	}
}

func (m tuiModel) Init() tea.Cmd {
	return m.loadStatus()
}

func (m tuiModel) loadStatus() tea.Cmd {
	return func() tea.Msg {
		status, err := collectTUIStatus(m.ctx, m.store)
		if err != nil {
			return tuiStatusMsg{result: status.StatusResult, tls: status.TLS, err: err}
		}
		doctor, doctorErr := collectTUIDoctor(m.ctx, m.store)
		return tuiStatusMsg{result: status.StatusResult, tls: status.TLS, doctor: doctor, err: doctorErr}
	}
}

func (m tuiModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % 3
		case "shift+tab", "left", "h":
			m.tab = (m.tab + 2) % 3
		case "r":
			m.loading = true
			m.err = nil
			return m, m.loadStatus()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tuiStatusMsg:
		m.loading = false
		m.result = msg.result
		m.tls = msg.tls
		m.doctor = msg.doctor
		m.err = msg.err
	}
	return m, nil
}

func (m tuiModel) View() tea.View {
	var b strings.Builder
	width := m.width
	if width <= 0 {
		width = 100
	}
	contentWidth := width - 4
	if contentWidth < 40 {
		contentWidth = width
	}

	titleStyle := lipgloss.NewStyle().Bold(true)
	activeTab := lipgloss.NewStyle().Bold(true).Underline(true)
	dim := lipgloss.NewStyle().Faint(true)
	success := lipgloss.NewStyle().Bold(true)
	failure := lipgloss.NewStyle().Bold(true)
	if !m.noColor {
		titleStyle = titleStyle.Foreground(lipgloss.Color("12"))
		activeTab = activeTab.Foreground(lipgloss.Color("14"))
		success = success.Foreground(lipgloss.Color("10"))
		failure = failure.Foreground(lipgloss.Color("9"))
	}

	title := "BaseHarbor"
	if m.result.Application != "" {
		title += " · " + m.result.Application
	}
	if m.result.Environment != "" {
		title += " · " + m.result.Environment
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	tabs := []string{"Overview", "Status", "Doctor"}
	for i, tab := range tabs {
		if i > 0 {
			b.WriteString("   ")
		}
		if i == m.tab {
			b.WriteString(activeTab.Render(tab))
		} else {
			b.WriteString(tab)
		}
	}
	b.WriteString("\n")
	b.WriteString(dim.Render(strings.Repeat("─", max(1, min(contentWidth, 80)))))
	b.WriteString("\n\n")

	switch {
	case m.loading:
		if m.reducedMotion {
			b.WriteString("Refreshing status...\n")
		} else {
			b.WriteString("Refreshing status…\n")
		}
	case m.err != nil:
		b.WriteString(failure.Render("FAILED"))
		b.WriteString("  ")
		b.WriteString(wrapTUIText(m.err.Error(), contentWidth-8))
		b.WriteString("\n\nNext:\n  baha doctor --verbose\n")
	case m.tab == 0:
		b.WriteString(renderTUISummary(m.result, contentWidth, success, failure))
	case m.tab == 1:
		b.WriteString(renderTUIOverviewWithTLS(m.result, m.tls, contentWidth, success, failure))
	default:
		b.WriteString(renderTUIDoctor(m.doctor, contentWidth, success, failure))
	}

	b.WriteString("\n")
	b.WriteString(dim.Render("Tab/←/→ switch · r refresh · q quit"))
	b.WriteString("\n")

	view := tea.NewView(b.String())
	view.AltScreen = true
	return view
}

func collectTUIStatus(ctx context.Context, store application.Store) (tuiApplicationStatusResult, error) {
	var out bytes.Buffer
	err := appStatusCommandWithTLS(store).Run(ctx, []string{"-o", "json"}, &out, io.Discard)
	var result tuiApplicationStatusResult
	if decodeErr := json.Unmarshal(out.Bytes(), &result); decodeErr != nil {
		if err != nil {
			return tuiApplicationStatusResult{}, errors.Join(err, decodeErr)
		}
		return tuiApplicationStatusResult{}, decodeErr
	}
	return result, nil
}

func collectTUIDoctor(ctx context.Context, store application.Store) (tuiDoctorResult, error) {
	var out bytes.Buffer
	err := appDoctorCommand(store).Run(ctx, []string{"-o", "json"}, &out, io.Discard)
	var result tuiDoctorResult
	if decodeErr := json.Unmarshal(out.Bytes(), &result); decodeErr != nil {
		if err != nil {
			return tuiDoctorResult{}, errors.Join(err, decodeErr)
		}
		return tuiDoctorResult{}, decodeErr
	}
	return result, nil
}

func renderTUISummary(result application.StatusResult, width int, success, failure lipgloss.Style) string {
	state := "READY"
	style := success
	if result.State == "stopped" {
		state = "STOPPED"
		style = failure
	} else if !result.Ready {
		state = "DEGRADED"
		style = failure
	}
	passed, failed := 0, 0
	for _, check := range result.Checks {
		if check.OK {
			passed++
		} else {
			failed++
		}
	}
	var b strings.Builder
	b.WriteString(style.Render(state))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Application   %s\n", result.Application)
	fmt.Fprintf(&b, "Environment   %s\n", result.Environment)
	fmt.Fprintf(&b, "Project       %s\n", result.Project)
	fmt.Fprintf(&b, "Checks        %d passed · %d failed\n", passed, failed)
	if !result.Ready && result.State != "stopped" {
		b.WriteString("\nNext\n  baha doctor\n  baha status --verbose\n")
	}
	return wrapTUIBlock(b.String(), width)
}

func renderTUIOverview(result application.StatusResult, width int, success, failure lipgloss.Style) string {
	return renderTUIOverviewWithTLS(result, nil, width, success, failure)
}

func renderTUIOverviewWithTLS(result application.StatusResult, tlsObservation *applicationTLSObservation, width int, success, failure lipgloss.Style) string {
	var b strings.Builder
	state := "READY"
	if result.State == "stopped" {
		state = "STOPPED"
	} else if !result.Ready {
		state = "DEGRADED"
	}
	if state == "READY" {
		b.WriteString(success.Render(state))
	} else {
		b.WriteString(failure.Render(state))
	}
	b.WriteString("\n\n")

	groups := []struct {
		name   string
		checks []application.StatusCheck
	}{
		{name: "Services"},
		{name: "Workload"},
		{name: "Observability"},
		{name: "Exposure"},
		{name: "Other"},
	}
	for _, check := range result.Checks {
		index := 4
		switch {
		case strings.HasPrefix(check.Name, "postgres"), strings.HasPrefix(check.Name, "valkey"), strings.HasPrefix(check.Name, "secrets"), strings.HasPrefix(check.Name, "runtime-broker"), strings.HasPrefix(check.Name, "object-storage"), strings.HasPrefix(check.Name, "required-secret"):
			index = 0
		case strings.HasPrefix(check.Name, "workload"):
			index = 1
		case strings.HasPrefix(check.Name, "logs"), strings.HasPrefix(check.Name, "telemetry"), strings.HasPrefix(check.Name, "traces"), strings.HasPrefix(check.Name, "metrics"):
			index = 2
		case strings.Contains(check.Name, "exposure"):
			index = 3
		}
		groups[index].checks = append(groups[index].checks, check)
	}

	for _, group := range groups {
		if len(group.checks) == 0 {
			continue
		}
		b.WriteString(group.name)
		b.WriteString("\n")
		for _, check := range group.checks {
			state := "READY"
			if group.name == "Observability" {
				state = "VERIFIED"
			}
			style := success
			if !check.OK {
				state = "FAILED"
				style = failure
			}
			line := fmt.Sprintf("  %-10s %-20s", state, check.Name)
			detail := statusHumanDetailValue(check, false)
			if strings.TrimSpace(detail) != "" {
				line += " " + detail
			}
			b.WriteString(style.Render(wrapTUIText(line, width)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if tlsObservation != nil {
		b.WriteString("TLS\n")
		b.WriteString(renderTUITLSObservation(*tlsObservation, width, success, failure))
		b.WriteString("\n")
	}
	if !result.Ready && result.State != "stopped" {
		b.WriteString("Next\n  baha doctor\n  baha status --verbose\n")
	}
	return b.String()
}

func renderTUIDoctor(result tuiDoctorResult, width int, success, failure lipgloss.Style) string {
	var b strings.Builder
	state := "READY"
	style := success
	if !result.Healthy {
		state = "DEGRADED"
		style = failure
	}
	b.WriteString(style.Render(state))
	b.WriteString("\n\n")
	if len(result.Checks) == 0 {
		b.WriteString("No doctor checks are currently available.\n")
		return b.String()
	}
	for _, check := range result.Checks {
		checkState := "OK"
		checkStyle := success
		if !check.OK {
			checkState = "FAILED"
			checkStyle = failure
		}
		line := fmt.Sprintf("%-8s %-24s", checkState, check.Name)
		detail := doctorHumanDetailValue(check, false)
		if strings.TrimSpace(detail) != "" {
			line += " " + detail
		}
		b.WriteString(checkStyle.Render(wrapTUIText(line, width)))
		b.WriteString("\n")
	}
	if result.TLS != nil {
		b.WriteString("\nTLS\n")
		b.WriteString(renderTUITLSObservation(*result.TLS, width, success, failure))
	}
	if !result.Healthy {
		b.WriteString("\nNext\n  baha doctor --verbose\n  baha status --verbose\n  baha app doctor --fix\n")
	}
	return b.String()
}

func renderTUITLSObservation(observation applicationTLSObservation, width int, success, failure lipgloss.Style) string {
	state := "READY"
	style := success
	if !observation.Healthy {
		state = "FAILED"
		style = failure
	}
	detail := strings.TrimSpace(observation.Detail)
	if detail == "" {
		detail = observation.Mode
	}
	var b strings.Builder
	line := fmt.Sprintf("  %-10s %-20s %s", state, "certificate", detail)
	b.WriteString(style.Render(wrapTUIText(line, width)))
	b.WriteString("\n")
	if observation.SourceState == "unavailable" && observation.Healthy {
		line = fmt.Sprintf("  %-10s %-20s %s", "WARN", "certificate-source", observation.Detail)
		b.WriteString(wrapTUIText(line, width))
		b.WriteString("\n")
	}
	return b.String()
}

func wrapTUIBlock(text string, width int) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = wrapTUIText(line, width)
	}
	return strings.Join(lines, "\n")
}

func wrapTUIText(text string, width int) string {
	if width < 30 {
		width = 30
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		if len([]rune(line))+1+len([]rune(word)) > width {
			lines = append(lines, line)
			line = "    " + word
			continue
		}
		line += " " + word
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n")
}
