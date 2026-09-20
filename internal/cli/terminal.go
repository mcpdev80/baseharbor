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
	Quiet         bool
	Verbose       bool
	NoColor       bool
	ReducedMotion bool
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

func NewTerminal(ctx context.Context, out, errOut io.Writer) *Terminal {
	opts := OutputOptionsFromContext(ctx)
	tty := writerIsTerminal(errOut)
	color := tty && !opts.NoColor && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return &Terminal{out: out, errOut: errOut, opts: opts, tty: tty, color: color}
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

func (t *Terminal) Quiet() bool   { return t.opts.Quiet }
func (t *Terminal) Verbose() bool { return t.opts.Verbose }
func (t *Terminal) TTY() bool     { return t.tty }

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

func (t *Terminal) Success(label, detail string) { t.state("success", label, detail) }
func (t *Terminal) Info(label, detail string)    { t.state("info", label, detail) }
func (t *Terminal) Warn(label, detail string)    { t.state("warning", label, detail) }
func (t *Terminal) Fail(label, detail string)    { t.state("failure", label, detail) }
func (t *Terminal) Skip(label, detail string)    { t.state("skipped", label, detail) }

func (t *Terminal) state(kind, label, detail string) {
	if t.opts.Quiet && kind != "failure" {
		return
	}
	symbol, plain, color := stateStyle(kind)
	marker := plain
	if t.tty {
		marker = symbol
	}
	if t.color {
		marker = color + marker + "\x1b[0m"
	}
	if strings.TrimSpace(detail) == "" {
		fmt.Fprintf(t.out, "  %s %s\n", marker, label)
		return
	}
	fmt.Fprintf(t.out, "  %s %-20s %s\n", marker, label, detail)
}

func stateStyle(kind string) (symbol, plain, color string) {
	switch kind {
	case "success":
		return "✓", "[OK]", "\x1b[32m"
	case "warning":
		return "!", "[WARN]", "\x1b[33m"
	case "failure":
		return "✗", "[FAIL]", "\x1b[31m"
	case "skipped":
		return "–", "[SKIP]", "\x1b[2m"
	default:
		return "•", "[INFO]", "\x1b[36m"
	}
}

// Activity runs potentially slow work with delayed progress feedback. It never
// invents percentages or ETAs. Successful buffered detail is shown only in
// verbose mode; failed detail is always revealed.
func (t *Terminal) Activity(ctx context.Context, label string, fn func(io.Writer) error) error {
	if t.opts.Quiet {
		return fn(io.Discard)
	}

	var buffer bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- fn(&buffer) }()

	delay := time.NewTimer(350 * time.Millisecond)
	defer delay.Stop()

	started := false
	var ticker *time.Ticker
	var ticks <-chan time.Time
	frames := []string{"◐", "◓", "◑", "◒"}
	frame := 0

	finish := func(err error) error {
		if ticker != nil {
			ticker.Stop()
		}
		if started {
			if t.tty && !t.opts.ReducedMotion {
				fmt.Fprint(t.errOut, "\r\x1b[2K")
			}
			if err == nil {
				t.activityLine("OK", label+" - done")
			} else {
				t.activityLine("FAIL", label+" - failed")
			}
		}
		if err != nil || t.opts.Verbose {
			_, _ = io.Copy(t.errOut, &buffer)
		}
		return err
	}

	for {
		select {
		case err := <-done:
			return finish(err)
		case <-ctx.Done():
			return finish(ctx.Err())
		case <-delay.C:
			started = true
			if t.tty && !t.opts.ReducedMotion {
				fmt.Fprintf(t.errOut, "\r%s %s", frames[0], label)
				ticker = time.NewTicker(800 * time.Millisecond)
				ticks = ticker.C
				frame = 1
			} else {
				t.activityLine("START", label)
			}
		case <-ticks:
			fmt.Fprintf(t.errOut, "\r%s %s", frames[frame%len(frames)], label)
			frame++
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
