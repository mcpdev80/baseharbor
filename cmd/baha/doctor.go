package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/health"
	"github.com/mcpdev80/baseharbor/internal/hosttrust"
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
	filtered, format, err := parseReadOutputArgs(args, "doctor")
	if err != nil {
		return err
	}
	args = filtered
	fix := false
	approved := false
	for _, arg := range args {
		switch arg {
		case "--fix":
			fix = true
		case "--yes":
			approved = true
		default:
			return usageError("unknown argument "+arg, "Usage: baha doctor [--fix --yes]")
		}
	}

	if approved && !fix {
		return usageError("--yes requires --fix", "Use baha doctor --fix --yes to confirm reconvergence of existing Core components.")
	}
	if format == outputJSON {
		if fix {
			return usageError("structured doctor output is read-only", "Use the explicit control-plane.repair semantic operation or human doctor --fix.")
		}
		result, err := inspectControlPlaneDoctor(ctx)
		if err != nil {
			return err
		}
		return writeJSON(out, result)
	}
	term := cli.NewTerminal(ctx, out, errOut)
	term.Header("Doctor", "")
	if err := authorizeCurrentMCPContext(ctx, "control-plane.doctor", "", "", ""); err != nil {
		return err
	}
	checks := collectControlPlaneDoctorChecks(ctx)
	ok := renderControlPlaneDoctor(term, checks)
	if ok {
		fmt.Fprintln(out, "\nREADY")
		return nil
	}

	findings := classifyDoctorFindings(checks)
	renderDoctorFindings(term, findings)
	if !fix {
		fmt.Fprintln(out, "\nNext:")
		if hasAutoFixableDoctorFinding(findings) {
			fmt.Fprintln(out, "  baha doctor --fix")
		}
		fmt.Fprintln(out, "  baha doctor --verbose")
		fmt.Fprintf(out, "\nDEGRADED · %d problem(s) require attention\n", len(findings))
		return cli.Presented(errors.New("one or more checks failed"))
	}

	if hasAutoFixableDoctorFinding(findings) {
		if !approved {
			return usageError("doctor --fix requires explicit consent before modifying the Core", "Review the findings and run baha doctor --fix --yes. Only existing runtime components will be reconverged.")
		}
		term.Section("Repair")
		if err := repairExistingControlPlaneRuntime(ctx, out); err != nil {
			term.Result("FAILED", "repair", err.Error())
		} else {
			term.Result("UPDATED", "repair", "reconverged existing control-plane runtime")
		}
	}

	after := collectControlPlaneDoctorChecks(ctx)
	term.Section("After repair")
	afterOK := renderControlPlaneDoctor(term, after)
	if afterOK {
		fmt.Fprintln(out, "\nREADY")
		return nil
	}

	remaining := classifyDoctorFindings(after)
	renderDoctorFindings(term, remaining)
	fmt.Fprintln(out, "\nNext:")
	fmt.Fprintln(out, "  Resolve the remaining problems above.")
	fmt.Fprintln(out, "  baha doctor --verbose")
	fmt.Fprintf(out, "\nDEGRADED · %d problem(s) still require attention\n", len(remaining))
	return cli.Presented(errors.New("one or more checks still require action"))
}

func appendControlPlaneAvailabilityDoctor(ctx context.Context, checks []health.Check) []health.Check {
	checks = appendHostTrustOwnershipDoctor(checks)
	report, err := collectControlPlaneAvailability(ctx, checks)
	if err != nil {
		return checks
	}
	return append(checks, health.Check{
		Name:    "control-plane-ha",
		OK:      report.Satisfied,
		Message: report.Detail(),
	})
}

func renderControlPlaneDoctor(term *cli.Terminal, checks []health.Check) bool {
	ok := true
	term.Section("Core")
	for _, check := range checks {
		state := "OK"
		if !check.OK {
			state = "FAILED"
			ok = false
		}
		detail := ""
		if !check.OK || term.Verbose() {
			detail = check.Message
		}
		term.Result(state, check.Name, detail)
	}
	return ok
}

func classifyDoctorFindings(checks []health.Check) []doctorFinding {
	findings := make([]doctorFinding, 0)
	for _, check := range checks {
		if check.OK {
			continue
		}
		finding := doctorFinding{Check: check, Class: doctorManualAction, Action: "inspect the failed prerequisite and correct it manually"}
		switch check.Name {
		case "target-selection":
			finding.Class = doctorNeedsInput
			finding.Action = "run baha target list, then baha target activate NAME"
		case "selected-runtime":
			finding.Action = "start or configure the selected runtime; rerun baha doctor"
		case "target-access":
			finding.Class = doctorNeedsInput
			finding.Action = "reconnect the authenticated remote Target and rerun baha doctor"
		case "host-trust-ownership":
			finding.Class = doctorManualAction
			finding.Action = "inspect the host-trust ownership state; do not delete or overwrite unverified records. If ownership is valid, use 'baha trust uninstall --yes' for explicit cleanup."
		case "container-runtime":
			finding.Action = "start or install Docker/Podman, then rerun 'baha doctor'"
		case "compose":
			finding.Action = "enable the Compose integration for Docker"
		case "quadlet":
			finding.Action = "install or enable Podman Quadlet and the user systemd session"
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

func renderDoctorFindings(term *cli.Terminal, findings []doctorFinding) {
	if len(findings) == 0 {
		return
	}
	term.Section("Problems")
	for _, finding := range findings {
		state := "FAILED"
		if finding.Class == doctorAutoFixable {
			state = "REPAIRABLE"
		}
		term.Result(state, finding.Check.Name, finding.Action)
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
	if err := authorizeCurrentMCPContext(parent, "control-plane.repair", "", "", ""); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	files, err := existingTargetRuntimeFiles(ctx)
	if err != nil {
		return fmt.Errorf("runtime is not initialized: %w", err)
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	runtimeProvider, err := detectRuntimeForTarget(ctx, target)
	if err != nil {
		return err
	}
	if err := runtimeProvider.Config(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	if err := runtimeProvider.Up(ctx, files.Compose, files.Env); err != nil {
		return err
	}
	fmt.Fprintln(out, "Repair applied: existing runtime definition converged without changing configuration.")
	return nil
}

func appendHostTrustOwnershipDoctor(checks []health.Check) []health.Check {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return append(checks, health.Check{Name: "host-trust-ownership", OK: false, Message: "host trust state directory unavailable: " + err.Error()})
	}
	records, err := hosttrust.StateRecords(dataDir)
	if err != nil {
		return append(checks, health.Check{Name: "host-trust-ownership", OK: false, Message: "host trust ownership state invalid: " + err.Error()})
	}
	if len(records) == 0 {
		return append(checks, health.Check{Name: "host-trust-ownership", OK: true, Message: "no BaseHarbor-owned host CA anchors"})
	}
	return append(checks, health.Check{Name: "host-trust-ownership", OK: true, Message: fmt.Sprintf("%d recorded BaseHarbor-owned CA anchor(s); inspect with 'baha trust status', remove explicitly with 'baha trust uninstall'", len(records))})
}
