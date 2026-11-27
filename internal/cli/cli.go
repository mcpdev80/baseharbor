package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
	return c.CommandChild(name)
}

// CommandChild resolves one routing step (a child name or alias) exactly the
// way Execute does, so tests and documentation checks share the first-match
// semantics instead of re-implementing the walk.
func (c *Command) CommandChild(name string) *Command {
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

// HelpSurface renders the --help page of this command and of every public
// descendant, and returns each page together with the exact argument vector
// that reaches it through Execute. It is the single shared traversal for help
// consumers: the golden help guard (card 0428-306) and every command-tree
// inventory derive their path sets from this one router-faithful walk instead
// of re-implementing child, alias, hidden and duplicate-name filtering.
//
// Routing rules it reproduces exactly:
//   - a Hidden command has no public help page and contributes no entry;
//   - a second registration of an already-visible name or alias is shadowed by
//     the first one (Execute's find returns the first match), so the shadowed
//     command contributes no entry, while its alias still resolves to the
//     first command's page;
//   - every public page is reachable as `baha <sub path> --help`; below the
//     root, the routed `baha help <sub path>` form is exposed as well.
//
// Entries come back sorted by path, so the result is deterministic.
func (c *Command) HelpSurface() []HelpSurfaceEntry {
	var entries []HelpSurfaceEntry
	var walk func(*Command, []string)
	seen := map[string]bool{}
	walk = func(command *Command, names []string) {
		if command == nil || command.Hidden {
			return
		}
		path := strings.Join(names, " ")
		if seen[path] {
			return // shadowed duplicate registration: no public page of its own
		}
		seen[path] = true
		var out bytes.Buffer
		command.renderHelp(&out)
		entries = append(entries, HelpSurfaceEntry{Path: path, Text: out.String(), Invocations: helpInvocations(c, names)})
		for _, child := range command.Children {
			walk(child, append(append([]string{}, names...), child.Name))
		}
	}
	walk(c, []string{c.Name})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

// helpInvocations returns the argument vectors that print one command page.
// The first is always the flag form `baha <sub path> --help`, which Execute
// answers for every command, the root included. The second form, the routed
// `baha help <sub path>`, is proven through the real router instead of
// assumed: Execute resolves it with its own first-match find, so the vector is
// exposed only when that routing reaches exactly this command (the root has no
// help-path form). Commands reachable through an alias print the same page as
// their canonical path, which the alias-walk cases in the shared-walk unit
// test lock down.
func helpInvocations(root *Command, names []string) [][]string {
	flag := append(append([]string{}, names[1:]...), "--help")
	if len(names) == 1 {
		return [][]string{flag}
	}
	target := root
	for _, part := range names[1:] {
		target = target.CommandChild(part)
		if target == nil {
			return [][]string{flag}
		}
	}
	if current := root; current != nil {
		for _, part := range names[1:] {
			next := current.CommandChild(part)
			if next == nil {
				return [][]string{flag}
			}
			current = next
		}
		if current != target {
			return [][]string{flag}
		}
	}
	return [][]string{flag, append([]string{"help"}, names[1:]...)}
}

// HelpSurfaceEntry is one public command path, its exact rendered --help text
// and the Execute argument vectors that reach that page.
type HelpSurfaceEntry struct {
	Path        string
	Text        string
	Invocations [][]string
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
