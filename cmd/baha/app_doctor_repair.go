package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

type appDoctorFinding struct {
	Name   string
	Detail string
	Class  doctorRepairClass
	Action string
}

func appDoctorRepairCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "doctor",
		Summary: "Diagnose and safely repair an application's runtime",
		Usage:   "baha app doctor [NAME] [--fix]",
		Long:    "Runs the existing application doctor, classifies failures using the same repair classes as root doctor, and with --fix only invokes the normal guarded app apply lifecycle when every remaining failure is safely repairable. External secrets, manifest or permission problems, and platform prerequisites remain fail-closed.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			nameArgs, fix, err := parseAppDoctorRepairArgs(args)
			if err != nil {
				return err
			}

			var diagnostic bytes.Buffer
			diagnosticErr := appDoctorCommand(store).Run(ctx, nameArgs, &diagnostic, errOut)
			fmt.Fprint(out, diagnostic.String())
			if diagnosticErr == nil {
				return nil
			}

			findings := classifyAppDoctorOutput(diagnostic.String())
			printAppDoctorFindings(out, findings)
			if !fix {
				fmt.Fprintln(out, "next: run 'baha app doctor --fix' to repair supported safe findings")
				return diagnosticErr
			}
			if len(findings) == 0 {
				return diagnosticErr
			}
			if !allAppDoctorFindingsAutoFixable(findings) {
				return errors.New("application doctor found findings that require developer or manual action before safe repair")
			}

			fmt.Fprintln(out, "Applying safe repair through the normal application lifecycle...")
			if err := appApplyCommand(store).Run(ctx, nameArgs, out, errOut); err != nil {
				return fmt.Errorf("safe application repair failed: %w", err)
			}

			fmt.Fprintln(out, "After repair:")
			return appDoctorCommand(store).Run(ctx, nameArgs, out, errOut)
		},
	}
}

func parseAppDoctorRepairArgs(args []string) ([]string, bool, error) {
	fix := false
	nameArgs := make([]string, 0, 1)
	for _, arg := range args {
		switch arg {
		case "--fix":
			if fix {
				return nil, false, usageError("--fix may only be specified once", "Usage: baha app doctor [NAME] [--fix]")
			}
			fix = true
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, false, usageError("unknown argument "+arg, "Usage: baha app doctor [NAME] [--fix]")
			}
			if len(nameArgs) != 0 {
				return nil, false, usageError("baha app doctor accepts at most one application name", "Usage: baha app doctor [NAME] [--fix]")
			}
			nameArgs = append(nameArgs, arg)
		}
	}
	return nameArgs, fix, nil
}

func classifyAppDoctorOutput(output string) []appDoctorFinding {
	findings := make([]appDoctorFinding, 0)
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "[FAIL] ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "[FAIL] "))
		name, detail, _ := strings.Cut(rest, ":")
		name = strings.TrimSpace(name)
		detail = strings.TrimSpace(detail)

		finding := appDoctorFinding{
			Name:   name,
			Detail: detail,
			Class:  doctorManualAction,
			Action: "inspect the failed application prerequisite and correct it manually",
		}
		switch name {
		case "runtime state", "managed runtime definition", "running services", "repository workload", "postgres running", "postgres readiness", "valkey running", "valkey readiness", "runtime secret broker":
			finding.Class = doctorAutoFixable
			finding.Action = "reconverge the application through 'baha app apply'"
		case "required application secrets":
			if strings.Contains(output, "missing - user input required") || strings.Contains(output, "present but unusable") {
				finding.Class = doctorNeedsInput
				finding.Action = "set each listed external or unusable secret with 'baha app secret set NAME --stdin'"
			} else if strings.Contains(output, "missing - will be generated automatically") {
				finding.Class = doctorAutoFixable
				finding.Action = "generate explicitly declared application secrets through 'baha app apply'"
			} else {
				finding.Class = doctorNeedsInput
				finding.Action = "inspect required-secret status and provide the missing value explicitly"
			}
		case "OpenBao application scope":
			finding.Class = doctorAutoFixable
			finding.Action = "reconverge the application OpenBao scope through 'baha app apply'"
		case "OpenBao control-plane runtime":
			finding.Class = doctorNeedsInput
			finding.Action = "repair the BaseHarbor control plane first with 'baha doctor' or operator-held OpenBao recovery material"
		case "manifest", "supported desired services", "manifest permissions", "workload discovery", "runtime permissions", "container runtime + compose", "compose configuration":
			finding.Class = doctorManualAction
		}
		findings = append(findings, finding)
	}
	return findings
}

func printAppDoctorFindings(out io.Writer, findings []appDoctorFinding) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(out, "%d application problem(s) classified:\n", len(findings))
	for _, finding := range findings {
		fmt.Fprintf(out, "- %s: %s -> %s\n", finding.Class, finding.Name, finding.Action)
	}
}

func allAppDoctorFindingsAutoFixable(findings []appDoctorFinding) bool {
	for _, finding := range findings {
		if finding.Class != doctorAutoFixable {
			return false
		}
	}
	return true
}
