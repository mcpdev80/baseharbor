package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

type completionCandidate struct {
	Value       string
	Description string
}

var usageFlagPattern = regexp.MustCompile(`(^|[[:space:]\[|])(-[A-Za-z]|--[A-Za-z0-9][A-Za-z0-9-]*)([ =][A-Z][A-Z0-9_-]*)?`)

func completionCommand(root *cli.Command, stores ...application.Store) *cli.Command {
	return &cli.Command{
		Name:    "completion",
		Summary: "Generate shell completion for Bash, Zsh or Fish",
		Usage:   "baha completion bash|zsh|fish",
		Long:    "Prints shell completion code to stdout. Completion is deterministic, read-only and never invokes application mutation.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("completion requires exactly one shell", "Use: baha completion bash|zsh|fish")
			}
			switch args[0] {
			case "bash":
				fmt.Fprint(out, bashCompletionScript())
			case "zsh":
				fmt.Fprint(out, zshCompletionScript())
			case "fish":
				fmt.Fprint(out, fishCompletionScript())
			default:
				return usageError("unsupported shell "+args[0], "Supported shells: bash, zsh, fish")
			}
			return nil
		},
	}
}

func internalCompletionCommand(root *cli.Command, stores ...application.Store) *cli.Command {
	return &cli.Command{
		Name:   "__complete",
		Hidden: true,
		Usage:  "baha __complete [WORDS...]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			for _, candidate := range completeCommandLine(root, args, stores...) {
				fmt.Fprintf(out, "%s\t%s\n", candidate.Value, candidate.Description)
			}
			return nil
		},
	}
}

func completeCommandLine(root *cli.Command, words []string, stores ...application.Store) []completionCandidate {
	current := root
	consumed := make([]string, 0, len(words))
	for i := 0; i < len(words)-1; i++ {
		word := words[i]
		if strings.HasPrefix(word, "-") {
			consumed = append(consumed, word)
			continue
		}
		if child := visibleChild(current, word); child != nil {
			current = child
		}
		consumed = append(consumed, word)
	}

	partial := ""
	if len(words) > 0 {
		partial = words[len(words)-1]
	}

	if len(consumed) > 0 {
		previous := consumed[len(consumed)-1]
		switch previous {
		case "-e", "--environment":
			return filterCandidates([]completionCandidate{
				{Value: "dev", Description: "developer-friendly local environment"},
				{Value: "test", Description: "test environment"},
				{Value: "prod", Description: "production environment"},
			}, partial)
		case "-o", "--output":
			return filterCandidates([]completionCandidate{
				{Value: "human", Description: "human-readable output"},
				{Value: "json", Description: "machine-readable JSON output"},
			}, partial)
		}
	}

	candidates := make([]completionCandidate, 0)
	for _, child := range current.Children {
		if child.Hidden {
			continue
		}
		candidates = append(candidates, completionCandidate{Value: child.Name, Description: child.Summary})
	}
	for _, flag := range commandUsageFlags(current) {
		candidates = append(candidates, flag)
	}
	if len(stores) != 0 && completionAcceptsApplicationName(current) {
		if items, err := stores[0].List(); err == nil {
			for _, item := range items {
				candidates = append(candidates, completionCandidate{Value: item.Name, Description: "configured BaseHarbor application"})
			}
		}
	}
	candidates = append(candidates,
		completionCandidate{Value: "--help", Description: "show command help"},
		completionCandidate{Value: "--quiet", Description: "suppress progress and non-essential human output"},
		completionCandidate{Value: "--verbose", Description: "show diagnostic runtime details"},
		completionCandidate{Value: "--no-color", Description: "disable ANSI color"},
		completionCandidate{Value: "--plain", Description: "stable styling-free line-oriented output"},
		completionCandidate{Value: "--no-input", Description: "never prompt for interactive input"},
		completionCandidate{Value: "--version", Description: "print the BaseHarbor version"},
	)
	return filterCandidates(uniqueCandidates(candidates), partial)
}


func completionAcceptsApplicationName(command *cli.Command) bool {
	switch command.Name {
	case "show", "plan", "status", "doctor", "apply", "up", "down", "destroy", "backup", "env", "psql", "redis", "logs", "shell", "exec", "update", "tls":
		return true
	default:
		return false
	}
}

func visibleChild(command *cli.Command, name string) *cli.Command {
	for _, child := range command.Children {
		if child.Hidden {
			continue
		}
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

func commandUsageFlags(command *cli.Command) []completionCandidate {
	matches := usageFlagPattern.FindAllStringSubmatch(command.Usage, -1)
	result := make([]completionCandidate, 0, len(matches))
	for _, match := range matches {
		flag := strings.TrimSpace(match[2])
		if flag == "" {
			continue
		}
		description := "command option"
		switch flag {
		case "-e", "--environment":
			description = "select deployment environment"
		case "-o", "--output":
			description = "select output format"
		case "-y", "--yes":
			description = "accept safe defaults without prompting"
		case "-q", "--quiet", "--silent":
			description = "suppress progress and non-essential output"
		case "-v", "--verbose":
			description = "show diagnostic runtime details"
		}
		result = append(result, completionCandidate{Value: flag, Description: description})
	}
	return uniqueCandidates(result)
}

func uniqueCandidates(candidates []completionCandidate) []completionCandidate {
	seen := map[string]bool{}
	result := make([]completionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Value == "" || seen[candidate.Value] {
			continue
		}
		seen[candidate.Value] = true
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Value < result[j].Value })
	return result
}

func filterCandidates(candidates []completionCandidate, partial string) []completionCandidate {
	result := make([]completionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if partial == "" || strings.HasPrefix(candidate.Value, partial) {
			result = append(result, candidate)
		}
	}
	return result
}

func bashCompletionScript() string {
	return `_baha_completion() {
    local value
    COMPREPLY=()
    while IFS=$'\t' read -r value _; do
        COMPREPLY+=("$value")
    done < <(command baha __complete "${COMP_WORDS[@]:1}")
}
complete -o default -F _baha_completion baha
`
}

func zshCompletionScript() string {
	return `#compdef baha
_baha_completion() {
    local -a lines values descriptions
    local line value description
    lines=("${(@f)$(command baha __complete "${words[@]:2}")}")
    for line in "${lines[@]}"; do
        value="${line%%$'\t'*}"
        description="${line#*$'\t'}"
        values+=("$value")
        descriptions+=("$description")
    done
    compadd -d descriptions -- "${values[@]}"
}
compdef _baha_completion baha
`
}

func fishCompletionScript() string {
	return `function __baha_complete
    set -l tokens (commandline -opc)
    set -e tokens[1]
    set -a tokens (commandline -ct)
    command baha __complete $tokens
end
complete -c baha -f -a '(__baha_complete)'
`
}
