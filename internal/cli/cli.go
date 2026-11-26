package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"
)

// RunFunc executes a command after command routing and built-in help handling.
type RunFunc func(context.Context, []string, io.Writer, io.Writer) error

// Command describes one CLI command. Commands can be nested arbitrarily.
type Command struct {
	Name       string
	Hidden     bool
	Aliases    []string
	Summary    string
	Usage      string
	Long       string
	Examples   []string
	Run        RunFunc
	Children   []*Command
	HelpGroups []HelpGroup
}

// HelpGroup prioritizes task-oriented commands without hiding advanced commands.
type HelpGroup struct {
	Title    string
	Commands []string
}

// UsageError represents invalid user input. Callers should exit with code 2.
type UsageError struct {
	NoSuggestions bool `json:"-"`
	Message       string
	Hint          string
}

func (e *UsageError) Error() string { return e.Message }

// PresentedError marks a failure that was already rendered as a complete human
// result by the command. The process must still fail, but main must not append a
// second generic error block.
type PresentedError struct {
	Err error
}

func (e *PresentedError) Error() string {
	if e == nil || e.Err == nil {
		return "command failed"
	}
	return e.Err.Error()
}

func (e *PresentedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func Presented(err error) error {
	if err == nil {
		return nil
	}
	return &PresentedError{Err: err}
}

func IsPresented(err error) bool {
	var presented *PresentedError
	return errors.As(err, &presented)
}

// ExitCodeError preserves a subprocess status through the command tree while
// retaining the original error for normal rendering and diagnostics.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	if e == nil || e.Err == nil {
		return "command failed"
	}
	return e.Err.Error()
}

func (e *ExitCodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WithExitCode associates an operating-system exit status with an error.
func WithExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	if code < 1 || code > 255 {
		return err
	}
	return &ExitCodeError{Code: code, Err: err}
}

// ExitCode maps command errors to stable process exit codes: 0 success, 1 runtime failure, 2 usage error.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		return 2
	}
	var exitCode *ExitCodeError
	if errors.As(err, &exitCode) && exitCode.Code > 0 && exitCode.Code <= 255 {
		return exitCode.Code
	}
	return 1
}

// SignalExitCode maps the two signals the CLI signal handler registers
// (os.Interrupt, syscall.SIGTERM) to the documented process exit codes named in
// docs/reference/cli.md: os.Interrupt -> 130, syscall.SIGTERM -> 143. It is the
// single source of that mapping; cmd/baha/main.go's interruptibleProcessContext
// goroutine routes every registered signal through this function on the
// signal-handler exit path (the second signal that arrives while a command is
// still running). main.go:context.Canceled -> 130 (the interrupt's first
// cancel) is the ordinary interruption exit; the 130/143 mapping here is the
// documented SIGINT/SIGTERM contract for the handler's own exit path.
func SignalExitCode(sig os.Signal) int {
	if sig == syscall.SIGTERM {
		return 143
	}
	return 130
}

// ResolveSignalExitCode is the main-level contract entry cmd/baha/main.go uses to
// turn a signal delivered to the running process into the documented process
// exit code. interruptibleProcessContext routes every registered signal through
// it on its exit path, so a test driving this function drives the exact code
// path main uses rather than a local key lookup.
func ResolveSignalExitCode(sig os.Signal) int {
	return SignalExitCode(sig)
}

// ResolveProcessExitCode is the main-level contract entry cmd/baha/main.go uses
// to turn the error returned by the command tree into the documented process
// exit code. main runs the command tree and, on error, routes the returned
// error through this helper instead of inlining the mapping: a context.Canceled
// error from an interrupt becomes 130 (it BYPASSES ExitCode, which would map a
// bare context.Canceled to the runtime-failure code 1), a *UsageError becomes 2,
// a WithExitCode-tagged error becomes its carried status, and everything else
// falls through to ExitCode(err), which returns 1 for an unclassified error. The
// two signals the handler registers (os.Interrupt, syscall.SIGTERM) stay 130/143
// via ResolveSignalExitCode on the running-process exit path; this helper is the
// single source for the interrupt's own process exit.
func ResolveProcessExitCode(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return ExitCode(err)
}

// Execute routes args through the command tree and handles help consistently.
func (c *Command) Execute(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		if c.Run == nil || len(c.Children) > 0 {
			c.Help(out)
			return nil
		}
		return c.Run(ctx, nil, out, errOut)
	}

	if isHelp(args[0]) {
		c.Help(out)
		return nil
	}
	if args[0] == "help" {
		return c.helpPath(args[1:], out)
	}

	if child := c.find(args[0]); child != nil {
		return child.Execute(ctx, args[1:], out, errOut)
	}
	if len(c.Children) > 0 && c.Run == nil {
		hint := fmt.Sprintf("Run '%s --help' to see available commands.", c.Name)
		if suggestion := c.suggest(args[0]); suggestion != "" {
			hint = fmt.Sprintf("Did you mean '%s'?\n  Run '%s %s --help' for details.", suggestion, c.Name, suggestion)
		}
		return &UsageError{
			Message: fmt.Sprintf("unknown command %q for %s", args[0], c.Name),
			Hint:    hint,
		}
	}
	if c.Run == nil {
		return &UsageError{Message: fmt.Sprintf("%s does not accept arguments", c.Name)}
	}
	for _, arg := range args {
		if isHelp(arg) {
			c.Help(out)
			return nil
		}
	}
	err := c.Run(ctx, args, out, errOut)
	return c.decorateUsageError(args, err)
}

func (c *Command) helpPath(path []string, out io.Writer) error {
	current := c
	for _, part := range path {
		next := current.find(part)
		if next == nil {
			hint := fmt.Sprintf("Run '%s --help' to see available commands.", current.Name)
			if suggestion := current.suggest(part); suggestion != "" {
				hint = fmt.Sprintf("Did you mean '%s'?\n  Run '%s %s --help' for details.", suggestion, current.Name, suggestion)
			}
			return &UsageError{
				Message: fmt.Sprintf("unknown command %q for %s", part, current.Name),
				Hint:    hint,
			}
		}
		current = next
	}
	current.Help(out)
	return nil
}

func (c *Command) find(name string) *Command {
	for _, child := range c.Children {
		if child.Name == name {
			return child
		}
		for _, alias := range child.Aliases {
			if alias == name {
				return child
			}
		}
	}
	return nil
}

func (c *Command) decorateUsageError(args []string, err error) error {
	if err == nil {
		return nil
	}
	var usage *UsageError
	if !errors.As(err, &usage) || usage.NoSuggestions || !strings.Contains(strings.ToLower(usage.Message), "unknown") {
		return err
	}
	known := c.knownFlags()
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		flag := arg
		if before, _, ok := strings.Cut(flag, "="); ok {
			flag = before
		}
		if containsString(known, flag) {
			continue
		}
		if suggestion := nearest(flag, known, 3); suggestion != "" {
			copy := *usage
			base := strings.TrimSpace(copy.Hint)
			if base != "" {
				base += "\n  "
			}
			copy.Hint = base + "Did you mean '" + suggestion + "'?"
			return &copy
		}
	}
	return err
}

var helpUsageFlagPattern = regexp.MustCompile(`--[A-Za-z0-9][A-Za-z0-9-]*|-[A-Za-z]`)

func (c *Command) knownFlags() []string {
	flags := []string{"-h", "--help", "-q", "--quiet", "--silent", "-v", "--verbose", "--plain", "--no-color", "--no-input", "--non-interactive", "--version"}
	for _, flag := range helpUsageFlagPattern.FindAllString(c.Usage, -1) {
		if !containsString(flags, flag) {
			flags = append(flags, flag)
		}
	}
	return flags
}

func nearest(input string, candidates []string, maxDistance int) string {
	best := ""
	bestDistance := maxDistance + 1
	for _, candidate := range candidates {
		distance := levenshtein(strings.ToLower(input), strings.ToLower(candidate))
		if distance < bestDistance {
			bestDistance = distance
			best = candidate
		}
	}
	if bestDistance <= maxDistance {
		return best
	}
	return ""
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func isHelp(arg string) bool { return arg == "-h" || arg == "--help" }

// Help renders stable, human-readable command documentation.
func (c *Command) Help(w io.Writer) {
	var buffer bytes.Buffer
	c.renderHelp(&buffer)
	writeMaybePaged(w, buffer.String())
}

func (c *Command) renderHelp(w io.Writer) {
	width := terminalTextWidth()
	if c.Summary != "" {
		fmt.Fprintf(w, "%s - %s\n", c.Name, c.Summary)
	} else {
		fmt.Fprintln(w, c.Name)
	}
	if strings.TrimSpace(c.Long) != "" {
		fmt.Fprintln(w)
		writeWrapped(w, strings.TrimSpace(c.Long), width, "")
	}
	if c.Usage != "" {
		fmt.Fprintf(w, "\nUsage:\n  %s\n", c.Usage)
	}
	if len(c.Children) > 0 {
		if len(c.HelpGroups) > 0 {
			seen := make(map[string]bool)
			byName := make(map[string]*Command, len(c.Children))
			for _, child := range c.Children {
				if !child.Hidden {
					byName[child.Name] = child
				}
			}
			for _, group := range c.HelpGroups {
				var children []*Command
				for _, name := range group.Commands {
					if child, ok := byName[name]; ok && !seen[name] {
						seen[name] = true
						children = append(children, child)
					}
				}
				if len(children) > 0 {
					fmt.Fprintf(w, "\n%s:\n", group.Title)
					for _, child := range children {
						writeWrapped(w, child.Summary, width, fmt.Sprintf("  %-14s  ", child.Name))
					}
				}
			}
			var advanced []*Command
			for _, child := range c.Children {
				if !child.Hidden && !seen[child.Name] {
					advanced = append(advanced, child)
				}
			}
			if len(advanced) > 0 {
				fmt.Fprintln(w, "\nAdvanced and operator commands:")
				for _, child := range advanced {
					writeWrapped(w, child.Summary, width, fmt.Sprintf("  %-14s  ", child.Name))
				}
			}
		} else {
			fmt.Fprintln(w, "\nCommands:")
			nameWidth := 0
			for _, child := range c.Children {
				if !child.Hidden && len(child.Name) > nameWidth {
					nameWidth = len(child.Name)
				}
			}
			for _, child := range c.Children {
				if child.Hidden {
					continue
				}
				prefix := fmt.Sprintf("  %-*s  ", nameWidth, child.Name)
				writeWrapped(w, child.Summary, width, prefix)
			}
		}
	}
	if len(c.Examples) > 0 {
		fmt.Fprintln(w, "\nExamples:")
		for _, example := range c.Examples {
			fmt.Fprintf(w, "  %s\n", example)
		}
	}
	fmt.Fprintln(w, "\nGlobal options:")
	writeHelpOption(w, width, "-q, --quiet", "Suppress progress and non-essential human output")
	writeHelpOption(w, width, "--silent", "Alias for --quiet")
	writeHelpOption(w, width, "-v, --verbose", "Show diagnostic runtime details")
	writeHelpOption(w, width, "--plain", "Stable styling-free line-oriented human output")
	writeHelpOption(w, width, "--no-color", "Disable ANSI color")
	writeHelpOption(w, width, "--no-input", "Never prompt; fail with an actionable error instead")
	writeHelpOption(w, width, "--version", "Print the BaseHarbor version")
	fmt.Fprintln(w, "\nHelp:")
	fmt.Fprintln(w, "  Use '<command> --help' for command-specific help.")
}

func writeHelpOption(w io.Writer, width int, option, description string) {
	const optionWidth = 18
	prefix := fmt.Sprintf("  %-*s  ", optionWidth, option)
	writeWrapped(w, description, width, prefix)
}

func terminalTextWidth() int {
	const fallback = 100
	value := strings.TrimSpace(os.Getenv("COLUMNS"))
	if value == "" {
		return fallback
	}
	width, err := strconv.Atoi(value)
	if err != nil || width < 40 {
		return fallback
	}
	if width > 160 {
		return 160
	}
	return width
}

func writeWrapped(w io.Writer, text string, width int, prefix string) {
	if width < 40 {
		width = 40
	}
	available := width - utf8.RuneCountInString(prefix)
	if available < 20 {
		available = 20
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		fmt.Fprintln(w, prefix)
		return
	}
	line := prefix
	lineLen := utf8.RuneCountInString(prefix)
	continuation := strings.Repeat(" ", utf8.RuneCountInString(prefix))
	for _, word := range words {
		wordLen := utf8.RuneCountInString(word)
		separator := 0
		if lineLen > utf8.RuneCountInString(prefix) {
			separator = 1
		}
		if lineLen+separator+wordLen > width && lineLen > utf8.RuneCountInString(prefix) {
			fmt.Fprintln(w, line)
			line = continuation + word
			lineLen = utf8.RuneCountInString(continuation) + wordLen
			continue
		}
		if separator != 0 {
			line += " "
			lineLen++
		}
		line += word
		lineLen += wordLen
	}
	fmt.Fprintln(w, line)
}

func SuggestClosest(input string, candidates []string) string {
	best := ""
	bestDistance := 3
	for _, candidate := range candidates {
		distance := levenshtein(strings.ToLower(input), strings.ToLower(candidate))
		if distance < bestDistance {
			bestDistance = distance
			best = candidate
		}
	}
	if bestDistance <= 2 {
		return best
	}
	return ""
}

func (c *Command) suggest(input string) string {
	best := ""
	bestDistance := 3
	for _, child := range c.Children {
		if child.Hidden {
			continue
		}
		distance := levenshtein(strings.ToLower(input), strings.ToLower(child.Name))
		if distance < bestDistance {
			bestDistance = distance
			best = child.Name
		}
		for _, alias := range child.Aliases {
			distance = levenshtein(strings.ToLower(input), strings.ToLower(alias))
			if distance < bestDistance {
				bestDistance = distance
				best = child.Name
			}
		}
	}
	if bestDistance <= 2 {
		return best
	}
	return ""
}

func levenshtein(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	prev := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ra := range ar {
		cur := make([]int, len(br)+1)
		cur[0] = i + 1
		for j, rb := range br {
			cost := 0
			if ra != rb {
				cost = 1
			}
			del := prev[j+1] + 1
			ins := cur[j] + 1
			sub := prev[j] + cost
			cur[j+1] = minInt(del, ins, sub)
		}
		prev = cur
	}
	return prev[len(br)]
}

func minInt(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}
