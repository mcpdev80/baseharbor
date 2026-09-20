package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
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
		return &UsageError{
			Message: fmt.Sprintf("unknown command %q for %s", args[0], c.Name),
			Hint:    fmt.Sprintf("Run '%s --help' to see available commands.", c.Name),
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
			return &UsageError{
				Message: fmt.Sprintf("unknown command %q for %s", part, current.Name),
				Hint:    fmt.Sprintf("Run '%s --help' to see available commands.", current.Name),
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
	if c.Summary != "" {
		fmt.Fprintf(w, "%s - %s\n", c.Name, c.Summary)
	} else {
		fmt.Fprintln(w, c.Name)
	}
	if strings.TrimSpace(c.Long) != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(c.Long))
	}
	if c.Usage != "" {
		fmt.Fprintf(w, "\nUsage:\n  %s\n", c.Usage)
	}
	if len(c.Children) > 0 {
		fmt.Fprintln(w, "\nCommands:")
		width := 0
		for _, child := range c.Children {
			if child.Hidden {
				continue
			}
			if len(child.Name) > width {
				width = len(child.Name)
			}
		}
		for _, child := range c.Children {
			if child.Hidden {
				continue
			}
			fmt.Fprintf(w, "  %-*s  %s\n", width, child.Name, child.Summary)
		}
	}
	fmt.Fprintln(w, "\nHelp:\n  Use '<command> --help' for command-specific help.")
}
