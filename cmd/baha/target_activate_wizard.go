package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func guidedTargetActivation(ctx context.Context, out, errOut io.Writer) error {
	if noInput(ctx) || !readerIsTerminal(appNewInput) {
		return usageError("target activate needs a name in non-interactive mode", "Run baha target list and then baha target activate NAME.")
	}
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	names := cfg.TargetNames()
	if _, ok := cfg.Targets["local"]; !ok {
		names = append(names, "local")
	}
	sort.Strings(names)
	if len(names) == 0 {
		return usageError("no target available", "Run baha new target to create one.")
	}
	fmt.Fprintln(out, "Select a deployment Target:")
	for i, name := range names {
		fmt.Fprintf(out, "  %d  %s\n", i+1, name)
	}
	reader := bufio.NewReader(appNewInput)
	var selected string
	for {
		fmt.Fprint(out, "Choice (number, name, or cancel): ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil && strings.TrimSpace(line) == "" {
			return fmt.Errorf("read target selection: %w", readErr)
		}
		choice := strings.TrimSpace(line)
		if strings.EqualFold(choice, "cancel") || strings.EqualFold(choice, "q") {
			fmt.Fprintln(out, "Cancelled. No changes were made.")
			return nil
		}
		if index, convErr := strconv.Atoi(choice); convErr == nil {
			if index >= 1 && index <= len(names) {
				selected = names[index-1]
			}
		} else {
			for _, candidate := range names {
				if choice == candidate {
					selected = candidate
					break
				}
			}
		}
		if selected != "" {
			break
		}
		fmt.Fprintln(out, "Invalid selection. Choose a displayed name or number, or cancel.")
	}
	// Re-read configuration at submit time: a concurrent operator may have deleted this Target.
	latest, err := deployment.LoadConfig()
	if err != nil {
		return err
	}
	if selected != "local" {
		if _, ok := latest.Targets[selected]; !ok {
			return usageError("selected target is no longer configured", "Run baha target activate again to refresh the available Targets.")
		}
	}
	if err := writePersistedTarget(selected); err != nil {
		return err
	}
	fmt.Fprintf(out, "Active target: %s\n", selected)
	return nil
}
