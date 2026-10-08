package main

import (
 "bufio"
 "context"
 "fmt"
 "io"
 "strings"

 "github.com/mcpdev80/baseharbor/internal/cli"
 "github.com/mcpdev80/baseharbor/internal/deployment"
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
   return runGuidedNewLocalTarget(ctx, out, errOut)
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

func runGuidedNewLocalTarget(ctx context.Context, out, errOut io.Writer) error {
 if noInput(ctx) || !readerIsTerminal(appNewInput) {
  return usageError("target creation requires choices in non-interactive mode", "Use 'baha new target NAME --runtime-provider docker --access local --reference local' or run interactively.")
 }
 reader := bufio.NewReader(appNewInput)
 steps := []struct{ label, defaultValue string }{
  {"Target name", ""},
  {"Runtime (docker/podman)", "docker"},
  {"Scope", "default"},
  {"Make the default target? (yes/no)", "no"},
 }
 answers := make([]string, len(steps))
 for step := 0; step < len(steps); {
  current := steps[step]
  fallback := current.defaultValue
  if answers[step] != "" { fallback = answers[step] }
  if fallback == "" { fmt.Fprintf(out, "%s (back/cancel): ", current.label) } else {
   fmt.Fprintf(out, "%s [%s] (back/cancel): ", current.label, fallback)
  }
  value, err := reader.ReadString('\n')
  if err != nil { return fmt.Errorf("read target configuration: %w", err) }
  value = strings.TrimSpace(value)
  switch strings.ToLower(value) {
  case "cancel", "q":
   fmt.Fprintln(out, "Cancelled. No changes were made.")
   return nil
  case "back":
   if step > 0 { step-- }
   continue
  }
  if value == "" { value = fallback }
  switch step {
  case 0:
   if err := deployment.ValidateTargetName(value); err != nil {
    fmt.Fprintf(out, "Invalid target name: %v. Try again.\n", err)
    continue
   }
  case 1:
   if value != "docker" && value != "podman" {
    fmt.Fprintln(out, "Choose docker or podman. Remote/advanced targets use 'baha target create'.")
    continue
   }
  case 2:
   if value == "" { fmt.Fprintln(out, "Scope cannot be empty."); continue }
  case 3:
   if value != "yes" && value != "no" {
    fmt.Fprintln(out, "Choose yes or no.")
    continue
   }
  }
  answers[step] = value
  step++
 }
 args := []string{answers[0], "--runtime-provider", answers[1], "--access", "local", "--reference", "local", "--scope", answers[2]}
 if answers[3] == "yes" { args = append(args, "--default") }
 fmt.Fprintf(out, "Create target %s with %s runtime, local access and scope %s? [y/N]: ", answers[0], answers[1], answers[2])
 confirm, err := reader.ReadString('\n')
 if err != nil { return fmt.Errorf("read target creation confirmation: %w", err) }
 if strings.TrimSpace(strings.ToLower(confirm)) != "y" && strings.TrimSpace(strings.ToLower(confirm)) != "yes" {
  fmt.Fprintln(out, "Cancelled. No changes were made.")
  return nil
 }
 return createTarget(ctx, args, out, errOut)
}
