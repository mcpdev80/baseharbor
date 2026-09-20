package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RunFunc executes a command after command routing and built-in help handling.
type RunFunc func(context.Context, []string, io.Writer, io.Writer) error

// Command describes one CLI command. Commands can be nested arbitrarily.
type Command struct {
	Name     string
	Hidden   bool
	Aliases  []string
	Summary  string
	Usage    string
	Long     string
	Examples []string
	Run      RunFunc
	Children []*Command
}

// UsageError represents invalid user input. Callers should exit with code 2.
type UsageError struct {
	Message string
	Hint    string
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

// ExitCode maps command errors to stable process exit codes: 0 success, 1 runtime failure, 2 usage error.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var usage *UsageError
	if errors.As(err, &usage) {
		return 2
	}
	return 1
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
	return c.Run(ctx, args, out, errOut)
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
		fmt.Fprintln(w, "\nCommands:")
		nameWidth := 0
		for _, child := range c.Children {
			if child.Hidden {
				continue
			}
			if len(child.Name) > nameWidth {
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
	if len(c.Examples) > 0 {
		fmt.Fprintln(w, "\nExamples:")
		for _, example := range c.Examples {
			fmt.Fprintf(w, "  %s\n", example)
		}
	}
	fmt.Fprintln(w, "\nGlobal options:")
	fmt.Fprintln(w, "  -q, --quiet           Suppress progress and non-essential human output")
	fmt.Fprintln(w, "      --silent          Alias for --quiet")
	fmt.Fprintln(w, "  -v, --verbose         Show diagnostic runtime details")
	fmt.Fprintln(w, "      --plain           Stable styling-free line-oriented human output")
	fmt.Fprintln(w, "      --no-color        Disable ANSI color")
	fmt.Fprintln(w, "      --no-input        Never prompt; fail with an actionable error instead")
	fmt.Fprintln(w, "      --version         Print the BaseHarbor version")
	fmt.Fprintln(w, "\nHelp:")
	fmt.Fprintln(w, "  Use '<command> --help' for command-specific help.")
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
