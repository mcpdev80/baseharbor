package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type outputOptionsKey struct{}

// OutputOptions controls human terminal rendering. Machine-readable command
// output must remain independent from these settings.
type OutputOptions struct {
	Quiet          bool
	Verbose        bool
	NoColor        bool
	ReducedMotion  bool
	Plain          bool
	NonInteractive bool
}

// WithOutputOptions attaches process-wide human-output preferences to a command
// context without changing command result models.
func WithOutputOptions(ctx context.Context, opts OutputOptions) context.Context {
	return context.WithValue(ctx, outputOptionsKey{}, opts)
}

// OutputOptionsFromContext returns the current human-output preferences.
func OutputOptionsFromContext(ctx context.Context) OutputOptions {
	opts, _ := ctx.Value(outputOptionsKey{}).(OutputOptions)
	return opts
}

// Terminal renders consistent human-facing CLI output. Primary command results
// still belong on stdout; activity and diagnostics belong on stderr.
type Terminal struct {
	out    io.Writer
	errOut io.Writer
	opts   OutputOptions
	tty    bool
	color  bool
	mu     sync.Mutex
}

type activityBuffer struct {
	bytes.Buffer
	details chan string
}

func newActivityBuffer() *activityBuffer {
	return &activityBuffer{details: make(chan string, 16)}
}

func (b *activityBuffer) ActivityDetail(detail string) {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return
	}
	select {
	case b.details <- detail:
	default:
		select {
		case <-b.details:
		default:
		}
		select {
		case b.details <- detail:
		default:
		}
	}
}

// ReportActivityDetail updates the human-facing detail of an in-flight Activity
// when w is the activity progress writer. Other writers intentionally ignore it.
func ReportActivityDetail(w io.Writer, detail string) {
	type reporter interface {
		ActivityDetail(string)
	}
	if r, ok := w.(reporter); ok {
		r.ActivityDetail(detail)
	}
}

func NewTerminal(ctx context.Context, out, errOut io.Writer) *Terminal {
	opts := OutputOptionsFromContext(ctx)
	tty := writerIsTerminal(errOut)
	if opts.Plain {
		opts.NoColor = true
		opts.ReducedMotion = true
	}
	color := tty && !opts.NoColor && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return &Terminal{out: out, errOut: errOut, opts: opts, tty: tty, color: color}
}

func IsTerminal(w io.Writer) bool {
	return writerIsTerminal(w)
}

func writerIsTerminal(w io.Writer) bool {
	type statWriter interface {
		Stat() (os.FileInfo, error)
	}
	file, ok := w.(statWriter)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (t *Terminal) Quiet() bool          { return t.opts.Quiet }
func (t *Terminal) Verbose() bool        { return t.opts.Verbose }
func (t *Terminal) Plain() bool          { return t.opts.Plain }
func (t *Terminal) NonInteractive() bool { return t.opts.NonInteractive }
func (t *Terminal) TTY() bool            { return t.tty && !t.opts.Plain }

func (t *Terminal) Header(app, environment string) {
	if t.opts.Quiet {
		return
	}
	title := "BaseHarbor"
	if strings.TrimSpace(app) != "" {
		title += " · " + app
	}
	if strings.TrimSpace(environment) != "" {
		title += " · " + environment
	}
	fmt.Fprintln(t.out, title)
}

func (t *Terminal) Section(title string) {
	if t.opts.Quiet {
		return
	}
	fmt.Fprintf(t.out, "\n%s\n", title)
}

func (t *Terminal) Success(label, detail string) { t.Result("OK", label, detail) }
func (t *Terminal) Info(label, detail string)    { t.Result("INFO", label, detail) }
func (t *Terminal) Warn(label, detail string)    { t.Result("WARN", label, detail) }
func (t *Terminal) Fail(label, detail string)    { t.Result("FAILED", label, detail) }
func (t *Terminal) Skip(label, detail string)    { t.Result("SKIPPED", label, detail) }

func (t *Terminal) Result(state, subject, detail string) {
	if t.opts.Quiet && state != "FAILED" {
		return
	}
	state = strings.ToUpper(strings.TrimSpace(state))
	if state == "" {
		state = "INFO"
	}
	const stateWidth = 10
	const subjectWidth = 24
	paddedStatePlain := fmt.Sprintf("%-*s", stateWidth, state)
	paddedState := paddedStatePlain
	if t.color {
		paddedState = stateColor(state) + paddedStatePlain + "\x1b[0m"
	}
	if strings.TrimSpace(detail) == "" {
		fmt.Fprintf(t.out, "  %s %-*s\n", paddedState, subjectWidth, subject)
		return
	}

	prefix := fmt.Sprintf("  %s %-*s ", paddedState, subjectWidth, subject)
	visiblePrefixWidth := 2 + stateWidth + 1 + subjectWidth + 1
	available := terminalTextWidth() - visiblePrefixWidth
	if available < 24 {
		available = 24
	}
	lines := wrapWords(strings.TrimSpace(detail), available)
	if len(lines) == 0 {
		fmt.Fprintln(t.out, strings.TrimRight(prefix, " "))
		return
	}
	fmt.Fprintln(t.out, prefix+lines[0])
	continuation := strings.Repeat(" ", visiblePrefixWidth)
	for _, line := range lines[1:] {
		fmt.Fprintln(t.out, continuation+line)
	}
}

func wrapWords(text string, width int) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if width < 8 {
		width = 8
	}
	words := strings.Fields(text)
	lines := make([]string, 0, 1)
	line := ""
	for _, word := range words {
		if line == "" {
			line = word
			continue
		}
		if len([]rune(line))+1+len([]rune(word)) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func stateColor(state string) string {
	switch state {
	case "FAILED", "ERROR":
		return "\x1b[31m"
	case "WARN", "WARNING", "RETRYING":
		return "\x1b[33m"
	case "READY", "VERIFIED", "CREATED", "UPDATED", "DELETED", "REMOVED", "STARTED", "STOPPED", "RESTORED", "BACKED UP", "OK":
		return "\x1b[32m"
	case "SKIPPED":
		return "\x1b[2m"
	default:
		return "\x1b[36m"
	}
}

// Activity runs potentially slow work with delayed progress feedback. It never
// invents percentages or ETAs. Successful buffered detail is shown only in
// verbose mode; failed detail is always revealed.
func (t *Terminal) Activity(ctx context.Context, label string, fn func(io.Writer) error) error {
	if t.opts.Quiet {
		var buffer bytes.Buffer
		err := fn(&buffer)
		if err != nil {
			_, _ = io.Copy(t.errOut, &buffer)
		}
		return err
	}

	progress := newActivityBuffer()
	done := make(chan error, 1)
	startedAt := time.Now()
	go func() { done <- fn(progress) }()

	delay := time.NewTimer(350 * time.Millisecond)
	defer delay.Stop()

	started := false
	latestDetail := ""
	lastPrintedDetail := ""
	var ticker *time.Ticker
	var ticks <-chan time.Time
	frames := []string{"◐", "◓", "◑", "◒"}
	frame := 0

	finish := func(err error) error {
		if ticker != nil {
			ticker.Stop()
		}
		elapsed := time.Since(startedAt)
		if started {
			if t.tty && !t.opts.ReducedMotion && !t.opts.Plain {
				fmt.Fprint(t.errOut, "\r\x1b[2K")
			}
			suffix := ""
			if elapsed >= time.Second {
				suffix = " (" + formatActivityDuration(elapsed) + ")"
			}
			if err == nil {
				t.activityLine("OK", label+" - done"+suffix)
			} else {
				t.activityLine("FAIL", label+" - failed"+suffix)
			}
		}
		if err != nil || t.opts.Verbose {
			_, _ = io.WriteString(t.errOut, progress.String())
		}
		return err
	}

	for {
		select {
		case err := <-done:
			return finish(err)
		case <-ctx.Done():
			return finish(ctx.Err())
		case detail := <-progress.details:
			latestDetail = detail
			if started {
				if t.tty && !t.opts.ReducedMotion && !t.opts.Plain {
					fmt.Fprintf(t.errOut, "\r\x1b[2K%s %s%s (%s)", frames[frame%len(frames)], label, formatActivityDetail(latestDetail), formatActivityDuration(time.Since(startedAt)))
					frame++
				} else if latestDetail != lastPrintedDetail {
					t.activityLine("INFO", label+" - "+latestDetail)
					lastPrintedDetail = latestDetail
				}
			}
		case <-delay.C:
			started = true
			if t.tty && !t.opts.ReducedMotion && !t.opts.Plain {
				fmt.Fprintf(t.errOut, "\r%s %s%s (%s)", frames[0], label, formatActivityDetail(latestDetail), formatActivityDuration(time.Since(startedAt)))
				ticker = time.NewTicker(800 * time.Millisecond)
				ticks = ticker.C
				frame = 1
			} else {
				t.activityLine("START", label)
				ticker = time.NewTicker(10 * time.Second)
				ticks = ticker.C
			}
		case <-ticks:
			if t.tty && !t.opts.ReducedMotion && !t.opts.Plain {
				fmt.Fprintf(t.errOut, "\r%s %s%s (%s)", frames[frame%len(frames)], label, formatActivityDetail(latestDetail), formatActivityDuration(time.Since(startedAt)))
				frame++
			} else {
				t.activityLine("WAIT", label+" - still working ("+formatActivityDuration(time.Since(startedAt))+")")
			}
		}
	}
}

func (t *Terminal) activityLine(state, label string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fmt.Fprintf(t.errOut, "[%s] %s\n", state, label)
}

func (t *Terminal) Diagnostic(format string, args ...any) {
	if !t.opts.Verbose {
		return
	}
	fmt.Fprintf(t.errOut, format, args...)
}


func formatActivityDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%02ds", minutes, seconds)
}


func formatActivityDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return ""
	}
	return " · " + detail
}
