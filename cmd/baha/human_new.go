package main

import (
 "bufio"
 "context"
 "fmt"
 "io"
 "strings"

 "github.com/mcpdev80/baseharbor/internal/cli"
)

// humanNewCommand is a presentation-only chooser. Domain mutation and policy
// checks remain in the existing creation commands.
func humanNewCommand() *cli.Command {
 return &cli.Command{
  Name: "new",
  Summary: "Create an application, stack, target, provider, or workspace",
  Usage: "baha new [application|stack|target|provider|workspace] [options]",
  Long: "Interactively choose the object to create, then continue through the existing domain flow. Use an explicit object type and options for automation.",
  Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
   if len(args) > 0 {
    return runHumanNewSelection(ctx, args[0], args[1:], out, errOut)
   }
   if noInput(ctx) || !readerIsTerminal(appNewInput) {
    return usageError("new requires a creation type without a terminal", "Use 'baha new application NAME', 'baha new stack', 'baha new target NAME', 'baha new provider ID', or 'baha new workspace'.")
   }
   fmt.Fprintln(out, "What do you want to create?")
   fmt.Fprintln(out, "  1  Application")
   fmt.Fprintln(out, "  2  Development stack")
   fmt.Fprintln(out, "  3  Target")
   fmt.Fprintln(out, "  4  Provider (authoring)")
   fmt.Fprintln(out, "  5  Workspace")
   fmt.Fprint(out, "Choice [1-5] (or cancel): ")
   line, err := bufio.NewReader(appNewInput).ReadString('\n')
   if err != nil && len(strings.TrimSpace(line)) == 0 {
    return fmt.Errorf("read creation choice: %w; next: use 'baha new --help' for explicit forms", err)
   }
   selection := strings.ToLower(strings.TrimSpace(line))
   if selection == "cancel" || selection == "q" {
    fmt.Fprintln(out, "Cancelled. No changes were made.")
    return nil
   }
   return runHumanNewSelection(ctx, selection, nil, out, errOut)
  },
 }
}

func runHumanNewSelection(ctx context.Context, kind string, args []string, out, errOut io.Writer) error {
 switch strings.ToLower(strings.TrimSpace(kind)) {
 case "1", "application", "app":
  return appNewCommand().Run(ctx, args, out, errOut)
 case "2", "stack", "development-stack":
  return stackCreateCommand().Run(ctx, args, out, errOut)
 case "3", "target":
  if len(args) == 0 {
   return usageError("target creation needs a deployment target definition", "Run 'baha target create --help' for the supported target access and scope choices.")
  }
  return createTarget(ctx, args, out, errOut)
 case "4", "provider":
  return providerInitCommand().Run(ctx, args, out, errOut)
 case "5", "workspace":
  return appWorkspaceCommand().Run(ctx, args, out, errOut)
 default:
  return usageError("unsupported creation type: "+kind, "Choose application, stack, target, provider, or workspace; see 'baha new --help'.")
 }
}
