package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func configCommand() *cli.Command {
	return &cli.Command{
		Name:    "config",
		Summary: "Configure BaseHarbor user preferences",
		Usage:   "baha config prompt [options]",
		Children: []*cli.Command{
			configPromptCommand(),
		},
	}
}

func configPromptCommand() *cli.Command {
	return &cli.Command{
		Name:    "prompt",
		Summary: "Configure the optional BaseHarbor shell prompt segment",
		Usage:   "baha config prompt [--enable|--disable] [--preset minimal|compact|accessible|detailed|none] [--position before-path|after-path|right] [--environment never|critical-only|always] [--show-application|--hide-application] [--text-only|--color-output] [--prod-indicator TEXT] [--label TARGET=LABEL] [--color KEY=VALUE]",
		Long:    "Configures prompt presentation only. Target identity, lifecycle ownership and deployment selection are never changed by prompt labels or colors. With no options, interactive terminals open a compact setup wizard with a live preview.",
		Run:     runConfigPrompt,
	}
}

func runConfigPrompt(ctx context.Context, args []string, out, errOut io.Writer) error {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	prompt := cfg.Prompt
	if prompt.Preset == "" {
		prompt.Preset = "compact"
	}
	if prompt.Position == "" {
		prompt.Position = "before-path"
	}
	if prompt.Environment == "" {
		prompt.Environment = "critical-only"
	}
	if prompt.Labels == nil {
		prompt.Labels = map[string]string{}
	}
	if prompt.Colors == nil {
		prompt.Colors = map[string]string{}
	}

	if len(args) == 0 {
		if noInput(ctx) {
			return usageError("config prompt requires explicit options in non-interactive mode", "Use 'baha config prompt --enable --preset compact' or run interactively.")
		}
		return runPromptWizard(cfg, prompt, out)
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--enable":
			prompt.Enabled = true
		case "--disable":
			prompt.Enabled = false
		case "--show-application":
			prompt.ShowApplication = true
		case "--hide-application":
			prompt.ShowApplication = false
		case "--text-only":
			prompt.TextOnly = true
		case "--color-output":
			prompt.TextOnly = false
		case "--preset", "--position", "--environment", "--prod-indicator", "--label", "--color":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return usageError(args[i]+" requires a value", "Run 'baha config prompt --help' for usage.")
			}
			key, value := args[i], strings.TrimSpace(args[i+1])
			i++
			switch key {
			case "--preset":
				if !oneOf(value, "minimal", "compact", "accessible", "detailed", "none") {
					return usageError("unsupported prompt preset "+value, "Use minimal, compact, accessible, detailed or none.")
				}
				prompt.Preset = value
				if value == "none" {
					prompt.Enabled = false
				}
			case "--position":
				if !oneOf(value, "before-path", "after-path", "right") {
					return usageError("unsupported prompt position "+value, "Use before-path, after-path or right.")
				}
				prompt.Position = value
			case "--environment":
				if !oneOf(value, "never", "critical-only", "always") {
					return usageError("unsupported environment display mode "+value, "Use never, critical-only or always.")
				}
				prompt.Environment = value
			case "--prod-indicator":
				prompt.ProdIndicator = value
			case "--label":
				target, label, ok := strings.Cut(value, "=")
				if !ok || strings.TrimSpace(target) == "" {
					return usageError("--label requires TARGET=LABEL", "Example: --label laptop-docker-12=ld12")
				}
				prompt.Labels[strings.TrimSpace(target)] = strings.TrimSpace(label)
			case "--color":
				name, color, ok := strings.Cut(value, "=")
				if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(color) == "" {
					return usageError("--color requires KEY=VALUE", "Example: --color dev=green")
				}
				prompt.Colors[strings.TrimSpace(name)] = strings.TrimSpace(color)
			}
		default:
			return unknownOptionUsage("baha config prompt", args[i], "--enable", "--disable", "--preset", "--position", "--environment", "--show-application", "--hide-application", "--text-only", "--color-output", "--prod-indicator", "--label", "--color")
		}
	}
	cfg.Prompt = prompt
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintln(out, promptPreview(cfg, prompt))
	fmt.Fprintln(out, "Prompt configuration saved.")
	return nil
}

func runPromptWizard(cfg deployment.Config, prompt deployment.PromptConfig, out io.Writer) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintln(out, "BaseHarbor prompt setup")
	fmt.Fprintln(out, promptPreview(cfg, prompt))

	prompt.Enabled = askPromptBool(reader, out, "Enable prompt integration", prompt.Enabled)
	prompt.Preset = askPromptChoice(reader, out, "Preset", prompt.Preset, "minimal", "compact", "accessible", "detailed", "none")
	prompt.Position = askPromptChoice(reader, out, "Position", prompt.Position, "before-path", "after-path", "right")
	prompt.Environment = askPromptChoice(reader, out, "Environment display", prompt.Environment, "never", "critical-only", "always")
	prompt.ShowApplication = askPromptBool(reader, out, "Show application when inside a repository", prompt.ShowApplication)
	prompt.TextOnly = askPromptBool(reader, out, "Use text-only prompt output", prompt.TextOnly)
	if target := promptPreviewTarget(cfg); target != "" {
		currentLabel := prompt.Labels[target]
		fmt.Fprintf(out, "Prompt label for %s [%s]: ", target, currentLabel)
		if value, _ := reader.ReadString('\n'); strings.TrimSpace(value) != "" {
			prompt.Labels[target] = strings.TrimSpace(value)
		}
	}
	fmt.Fprintf(out, "Production indicator [%s]: ", prompt.ProdIndicator)
	if value, _ := reader.ReadString('\n'); strings.TrimSpace(value) != "" {
		prompt.ProdIndicator = strings.TrimSpace(value)
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Preview:")
	fmt.Fprintln(out, promptPreview(cfg, prompt))
	if !askPromptBool(reader, out, "Save this configuration", true) {
		fmt.Fprintln(out, "Prompt configuration unchanged.")
		return nil
	}
	cfg.Prompt = prompt
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintln(out, "Prompt configuration saved.")
	return nil
}

func askPromptBool(reader *bufio.Reader, out io.Writer, label string, current bool) bool {
	def := "y/N"
	if current {
		def = "Y/n"
	}
	fmt.Fprintf(out, "%s [%s]: ", label, def)
	value, _ := reader.ReadString('\n')
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return current
	}
	return value == "y" || value == "yes"
}

func askPromptChoice(reader *bufio.Reader, out io.Writer, label, current string, allowed ...string) string {
	fmt.Fprintf(out, "%s [%s] (%s): ", label, current, strings.Join(allowed, "/"))
	value, _ := reader.ReadString('\n')
	value = strings.TrimSpace(value)
	if value == "" {
		return current
	}
	if oneOf(value, allowed...) {
		return value
	}
	return current
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func promptCommand() *cli.Command {
	return &cli.Command{
		Name:    "prompt",
		Hidden:  true,
		Summary: "Render the local BaseHarbor prompt segment",
		Usage:   "baha prompt [--plain|--position-only]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			plain := false
			positionOnly := false
			for _, arg := range args {
				switch arg {
				case "--plain":
					plain = true
				case "--position-only":
					positionOnly = true
				default:
					return unknownOptionUsage("baha prompt", arg, "--plain", "--position-only")
				}
			}
			cfg, err := deployment.LoadConfig()
			if err != nil {
				return err
			}
			if !cfg.Prompt.Enabled || cfg.Prompt.Preset == "none" {
				return nil
			}
			if positionOnly {
				position := strings.TrimSpace(cfg.Prompt.Position)
				if position == "" {
					position = "before-path"
				}
				fmt.Fprint(out, position)
				return nil
			}
			target, err := effectiveTarget(ctx)
			if err != nil {
				return nil
			}
			applicationName, environment := repositoryPromptContext()
			fmt.Fprint(out, renderPromptSegment(cfg.Prompt, target.Name, applicationName, environment, plain))
			return nil
		},
	}
}

func promptPreviewTarget(cfg deployment.Config) string {
	target := cfg.DefaultTarget
	if target == "" {
		for _, name := range cfg.TargetNames() {
			target = name
			break
		}
	}
	if target == "" {
		target = "docker-dev"
	}
	return target
}

func promptPreview(cfg deployment.Config, prompt deployment.PromptConfig) string {
	return "Preview: " + renderPromptSegment(prompt, promptPreviewTarget(cfg), "demo", "dev", true)
}

func renderPromptSegment(prompt deployment.PromptConfig, target, applicationName, environment string, plain bool) string {
	label := strings.TrimSpace(prompt.Labels[target])
	if label == "" {
		label = target
	}
	env := strings.ToLower(strings.TrimSpace(environment))
	envLabel := ""
	switch prompt.Environment {
	case "always":
		if env == "prod" || env == "production" {
			envLabel = strings.TrimSpace(prompt.ProdIndicator)
			if envLabel == "" {
				envLabel = "PROD"
			}
		} else if env != "" {
			envLabel = strings.ToUpper(env)
		}
	case "critical-only", "":
		if env == "prod" || env == "production" {
			envLabel = strings.TrimSpace(prompt.ProdIndicator)
			if envLabel == "" {
				envLabel = "PROD"
			}
		} else if env == "test" || env == "stage" || env == "staging" {
			envLabel = strings.ToUpper(env)
		}
	}
	text := "[" + label
	if prompt.ShowApplication && strings.TrimSpace(applicationName) != "" {
		text += "/" + strings.TrimSpace(applicationName)
	}
	if envLabel != "" {
		text += " " + envLabel
	}
	text += "]"
	if plain || prompt.TextOnly {
		return text
	}
	color := promptColor(prompt, env, target)
	if color == "" {
		return text
	}
	return "\x1b[" + color + "m" + text + "\x1b[0m"
}

func promptColor(prompt deployment.PromptConfig, environment, target string) string {
	if custom := strings.TrimSpace(prompt.Colors[target]); custom != "" {
		return ansiColor(custom)
	}
	if custom := strings.TrimSpace(prompt.Colors[environment]); custom != "" {
		return ansiColor(custom)
	}
	switch environment {
	case "prod", "production":
		return "31"
	case "test", "stage", "staging":
		return "33"
	default:
		return "32"
	}
}

func ansiColor(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "black":
		return "30"
	case "red":
		return "31"
	case "green":
		return "32"
	case "yellow", "amber", "orange":
		return "33"
	case "blue":
		return "34"
	case "magenta":
		return "35"
	case "cyan":
		return "36"
	case "white":
		return "37"
	default:
		if _, err := strconv.Atoi(value); err == nil {
			return value
		}
		return ""
	}
}

func repositoryPromptContext() (string, string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	selection, err := application.ResolveRepositoryEnvironment(cwd, applicationEnvironmentOverride)
	if err != nil {
		return "", ""
	}
	return selection.Manifest.Name, selection.Manifest.Environment
}

func shellInitCommand() *cli.Command {
	return &cli.Command{
		Name:    "shell-init",
		Summary: "Generate BaseHarbor shell integration for Bash, Zsh or Fish",
		Usage:   "baha shell-init bash|zsh|fish",
		Long:    "Generates shell-local target activation helpers and optional prompt integration. Prompt rendering reads only local config and repository metadata and never contacts a runtime.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			if len(args) != 1 {
				return usageError("shell-init requires exactly one shell", "Use: baha shell-init bash|zsh|fish")
			}
			switch args[0] {
			case "bash":
				fmt.Fprint(out, bashShellInit())
			case "zsh":
				fmt.Fprint(out, zshShellInit())
			case "fish":
				fmt.Fprint(out, fishShellInit())
			default:
				return usageError("unsupported shell "+args[0], "Supported shells: bash, zsh, fish")
			}
			return nil
		},
	}
}

func bashShellInit() string {
	return `baha_target_activate() { export BASEHARBOR_TARGET="$1"; }
baha_target_deactivate() { unset BASEHARBOR_TARGET; }
_baha_prompt_command() {
    local bh position base
    bh="$(command baha prompt 2>/dev/null || true)"
    position="$(command baha prompt --position-only 2>/dev/null || true)"
    if [ -n "$BH_BASE_PS1" ]; then
        base="$BH_BASE_PS1"
    else
        base="$PS1"
        BH_BASE_PS1="$PS1"
    fi
    case "$position" in
        after-path) PS1="$base $bh " ;;
        *)          PS1="$bh $base" ;;
    esac
    [ -z "$bh" ] && PS1="$base"
}
PROMPT_COMMAND="_baha_prompt_command${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
`
}

func zshShellInit() string {
	return `baha_target_activate() { export BASEHARBOR_TARGET="$1"; }
baha_target_deactivate() { unset BASEHARBOR_TARGET; }
autoload -Uz add-zsh-hook
_baha_prompt_command() {
    local bh position
    bh="$(command baha prompt 2>/dev/null)"
    position="$(command baha prompt --position-only 2>/dev/null)"
    if [[ "$position" == "right" ]]; then
        RPROMPT="$bh"
        PROMPT="${BH_BASE_PROMPT:-$PROMPT}"
    else
        RPROMPT=""
        [[ -z "$BH_BASE_PROMPT" ]] && BH_BASE_PROMPT="$PROMPT"
        if [[ "$position" == "after-path" ]]; then
            PROMPT="$BH_BASE_PROMPT $bh "
        else
            PROMPT="$bh $BH_BASE_PROMPT"
        fi
        [[ -z "$bh" ]] && PROMPT="$BH_BASE_PROMPT"
    fi
}
add-zsh-hook precmd _baha_prompt_command
`
}

func fishShellInit() string {
	return `function baha_target_activate
    set -gx BASEHARBOR_TARGET $argv[1]
end
function baha_target_deactivate
    set -e BASEHARBOR_TARGET
end
if functions -q fish_prompt; and not functions -q __baha_original_fish_prompt
    functions -c fish_prompt __baha_original_fish_prompt
    function fish_prompt
        set -l position (command baha prompt --position-only 2>/dev/null)
        set -l bh (command baha prompt 2>/dev/null)
        if test "$position" = "before-path"; and test -n "$bh"
            printf "%s " "$bh"
        end
        __baha_original_fish_prompt
        if test "$position" = "after-path"; and test -n "$bh"
            printf " %s " "$bh"
        end
    end
end
if functions -q fish_right_prompt; and not functions -q __baha_original_fish_right_prompt
    functions -c fish_right_prompt __baha_original_fish_right_prompt
    function fish_right_prompt
        set -l position (command baha prompt --position-only 2>/dev/null)
        if test "$position" = "right"
            command baha prompt 2>/dev/null
        else
            __baha_original_fish_right_prompt
        end
    end
else if not functions -q fish_right_prompt
    function fish_right_prompt
        set -l position (command baha prompt --position-only 2>/dev/null)
        if test "$position" = "right"
            command baha prompt 2>/dev/null
        end
    end
end
`
}

func shellActivationCode(shell, target string) string {
	switch shell {
	case "fish":
		return "set -gx BASEHARBOR_TARGET " + strconv.Quote(target) + "\n"
	default:
		return "export BASEHARBOR_TARGET=" + strconv.Quote(target) + "\n"
	}
}

func shellDeactivationCode(shell string) string {
	if shell == "fish" {
		return "set -e BASEHARBOR_TARGET\n"
	}
	return "unset BASEHARBOR_TARGET\n"
}

func currentShellName() string {
	base := filepath.Base(strings.TrimSpace(os.Getenv("SHELL")))
	switch base {
	case "fish":
		return "fish"
	case "zsh":
		return "zsh"
	default:
		return "bash"
	}
}
