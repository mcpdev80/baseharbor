package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/health"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type doctorRepairClass string

const (
	doctorAutoFixable       doctorRepairClass = "auto-fixable"
	doctorNeedsConfirmation doctorRepairClass = "fixable with confirmation"
	doctorNeedsInput        doctorRepairClass = "requires developer input"
	doctorManualAction      doctorRepairClass = "manual/admin action required"
)

type doctorFinding struct {
	Check  health.Check
	Class  doctorRepairClass
	Action string
}

func doctorCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	fix := false
	for _, arg := range args {
		switch arg {
		case "--fix":
			fix = true
		default:
			return usageError("unknown argument "+arg, "Usage: baha doctor [--fix]")
		}
	}

	checks := health.Doctor()
	formatted, ok := health.Format(checks)
	fmt.Fprint(out, formatted)
	if ok {
		return nil
	}

	findings := classifyDoctorFindings(checks)
	printDoctorFindings(out, findings)
	if !fix {
		fmt.Fprintln(out, "next: run 'baha doctor --fix' to repair supported safe findings")
		return errors.New("one or more checks failed")
	}

	if hasAutoFixableDoctorFinding(findings) {
		if err := repairExistingControlPlaneRuntime(ctx, out); err != nil {
			fmt.Fprintf(out, "[FAIL] repair             %v\n", err)
		} else {
			fmt.Fprintln(out, "[OK] repair             reconverged existing control-plane runtime")
		}
	}

	after := health.Doctor()
	afterFormatted, afterOK := health.Format(after)
	fmt.Fprintln(out, "After repair:")
	fmt.Fprint(out, afterFormatted)
	if afterOK {
		return nil
	}

	remaining := classifyDoctorFindings(after)
	printDoctorFindings(out, remaining)
	return errors.New("one or more checks still require action")
}

func classifyDoctorFindings(checks []health.Check) []doctorFinding {
	findings := make([]doctorFinding, 0)
	for _, check := range checks {
		if check.OK {
			continue
		}
		finding := doctorFinding{Check: check, Class: doctorManualAction, Action: "inspect the failed prerequisite and correct it manually"}
		switch check.Name {
		case "container-runtime":
			finding.Action = "start or install Docker/Podman, then rerun 'baha doctor'"
		case "compose":
			finding.Action = "enable the Compose integration for the selected container runtime"
		case "runtime-config":
			finding.Action = "repair or deliberately recreate the local BaseHarbor runtime configuration"
		case "postgres":
			finding.Class = doctorAutoFixable
			finding.Action = "reconverge the existing BaseHarbor control-plane runtime"
		case "openbao":
			switch {
			case strings.Contains(check.Message, "not reachable"):
				finding.Class = doctorAutoFixable
				finding.Action = "reconverge the existing BaseHarbor control-plane runtime"
			case strings.Contains(check.Message, "not initialized"):
				finding.Class = doctorNeedsInput
				finding.Action = "run 'baha openbao bootstrap --recovery-file PATH' with an operator-selected recovery path"
			case strings.Contains(check.Message, "sealed"):
				finding.Class = doctorNeedsInput
				finding.Action = "run 'baha openbao unseal --recovery-file PATH' with the operator-held recovery file"
			default:
				finding.Action = "inspect OpenBao health before making any security-sensitive change"
			}
		}
		findings = append(findings, finding)
	}
	return findings
}

func printDoctorFindings(out io.Writer, findings []doctorFinding) {
	if len(findings) == 0 {
		return
	}
	fmt.Fprintf(out, "%d problem(s) found:\n", len(findings))
	for _, finding := range findings {
		fmt.Fprintf(out, "- %s: %s -> %s\n", finding.Class, finding.Check.Name, finding.Action)
	}
}

func hasAutoFixableDoctorFinding(findings []doctorFinding) bool {
	for _, finding := range findings {
		if finding.Class == doctorAutoFixable {
			return true
		}
	}
	return false
}

func repairExistingControlPlaneRuntime(parent context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	files, err := bhruntime.ExistingFiles("")
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	compose, err := bhruntime.DetectCompose(ctx)
	if err != nil {
		return err
	}
	if err := compose.Config(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	if err := compose.Up(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	fmt.Fprintln(out, "Repair applied: existing runtime definition converged without changing configuration.")
	return nil
}
