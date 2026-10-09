package main

import (
    "context"
    "errors"
    "io"

    "github.com/mcpdev80/baseharbor/internal/cli"
)

// useTargetCommand exposes the existing guided/persisted target activation.
// The target command remains the single implementation of selection rules.
func useTargetCommand() *cli.Command {
    return &cli.Command{
        Name: "use",
        Summary: "Select and persist the active Core or deployment target",
        Usage: "baha use [TARGET]",
        Long: "Select a configured Target interactively or persist an explicit selection. Equivalent to baha target activate.",
        Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
            for _, child := range targetCommand().Children {
                if child.Name == "activate" && child.Run != nil {
                    return child.Run(ctx, args, out, errOut)
                }
            }
            return errors.New("target activation is unavailable")
        },
    }
}
