package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/machine"
	"github.com/mcpdev80/baseharbor/internal/preflight"
)

type appDoctorFinding struct {
	Name   string
	Detail string
	Class  doctorRepairClass
	Action string
}

type appDoctorStructuredResult struct {
	Healthy         bool               `json:"healthy"`
	Checks          []preflight.Result `json:"checks"`
	RequiredSecrets []struct {
		Name      string `json:"name"`
		Present   bool   `json:"present"`
		Usable    bool   `json:"usable"`
		Generated bool   `json:"generated"`
	} `json:"required_secrets,omitempty"`
}

func appDoctorRepairCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "doctor",
		Summary: "Diagnose and safely repair an application's runtime",
		Usage:   "baha app doctor [NAME] [--fix]",
		Long:    "Runs the existing application doctor, including required-secret presence/usability checks, classifies failures using the same repair classes as root doctor, and with --fix only invokes the normal guarded app apply lifecycle when every remaining failure is safely repairable. External secrets, manifest or permission problems, and platform prerequisites remain fail-closed.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			return executeApplicationRepairLifecycle(ctx, store, args, out, errOut)
		},
	}
}

func executeApplicationRepairLifecycle(ctx context.Context, store application.Store, args []string, out, errOut io.Writer) error {
	if requestsJSONOutput(args) {
		for _, arg := range args {
			if arg == "--fix" {
				return usageError("--fix cannot be combined with structured output", "Run doctor in human mode for guarded repair, or remove --fix for read-only JSON.")
			}
		}
		return appDoctorCommand(store).Run(ctx, args, out, errOut)
	}
	nameArgs, fix, err := parseAppDoctorRepairArgs(args)
	if err != nil {
		return err
	}

	doctor, err := collectApplicationDoctor(ctx, store, nameArgs)
	if err != nil {
		return err
	}
	renderCollectedApplicationDoctor(ctx, out, errOut, doctor)
	if doctor.Healthy || doctor.State == "not_applied" {
		return nil
	}

	if !fix {
		return cli.Presented(errors.New("application doctor found one or more failures"))
	}

	findings := classifyApplicationDoctor(doctor)
	printAppDoctorFindings(out, findings)
	if len(findings) == 0 {
		return errors.New("application doctor reported failure but no structured findings were available for safe repair")
	}
	if !allAppDoctorFindingsAutoFixable(findings) {
		return errors.New("application doctor found findings that require developer or manual action before safe repair")
	}

	fmt.Fprintln(out, "Applying safe repair through the normal application lifecycle...")
	if findingsNeedControlPlaneRepair(findings) {
		fmt.Fprintln(out, "Restoring existing BaseHarbor control-plane runtime...")
		if err := runtimeUpExisting(ctx, out, ""); err != nil {
			return fmt.Errorf("safe application repair could not restore the BaseHarbor control plane: %w", err)
		}
	}
	if err := executeApplicationApplyLifecycle(ctx, store, nameArgs, out, errOut); err != nil {
		return fmt.Errorf("safe application repair failed: %w", err)
	}

	fmt.Fprintln(out, "After repair:")
	after, err := collectApplicationDoctor(ctx, store, nameArgs)
	if err != nil {
		return err
	}
	renderCollectedApplicationDoctor(ctx, out, errOut, after)
	if !after.Healthy {
		return &machine.Error{
			Code:        machine.ErrorVerificationFailed,
			CauseCode:   "repair_verification_failed",
			Message:     "Application repair completed but verification is still degraded.",
			Resource:    after.Application,
			Remediation: "manual/admin action required",
			Next:        "Inspect baseharbor.doctor findings and resolve the remaining non-repairable condition.",
		}
	}
	return nil
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

func classifyApplicationDoctor(result applicationDoctorResult) []appDoctorFinding {
	findings := make([]appDoctorFinding, 0)
	requiredByName := make(map[string]struct {
		present   bool
		usable    bool
		generated bool
	}, len(result.RequiredSecrets))
	for _, secret := range result.RequiredSecrets {
		requiredByName[secret.Name] = struct {
			present   bool
			usable    bool
			generated bool
		}{present: secret.Present, usable: secret.Usable, generated: secret.Generated}
	}

	for _, check := range result.Checks {
		if check.OK {
			continue
		}
		finding := classifyAppDoctorFinding(check.Name, check.Detail, requiredByName)
		findings = append(findings, finding)
	}
	return findings
}

func classifyAppDoctorFinding(name, detail string, required map[string]struct {
	present   bool
	usable    bool
	generated bool
}) appDoctorFinding {
	finding := appDoctorFinding{
		Name:   name,
		Detail: detail,
		Class:  doctorManualAction,
		Action: "inspect the failed application prerequisite and correct it manually",
	}
	lowerDetail := strings.ToLower(detail)

	switch name {
	case "runtime state", "managed runtime definition", "running services", "repository workload", "postgres running", "postgres readiness", "valkey running", "valkey readiness", "runtime secret broker", "application runtime broker":
		finding.Class = doctorAutoFixable
		finding.Action = "reconverge the application through 'baha app apply'"
	case "workload security":
		if strings.Contains(lowerDetail, "managed by baseharbor/openbao") ||
			(strings.Contains(lowerDetail, "secret_key") && strings.Contains(lowerDetail, "required variable")) {
			finding.Class = doctorAutoFixable
			finding.Action = "reconverge BaseHarbor-managed workload secrets through 'baha app apply'"
		}
	case "required application secrets":
		if strings.Contains(lowerDetail, "openbao") &&
			(strings.Contains(lowerDetail, "not running") || strings.Contains(lowerDetail, "unavailable")) {
			finding.Class = doctorAutoFixable
			finding.Action = "reconverge the application OpenBao scope and re-check required secrets"
			break
		}
		hasNeedsInput := false
		hasGeneratedMissing := false
		for _, secret := range required {
			if secret.present && secret.usable {
				continue
			}
			if secret.generated {
				hasGeneratedMissing = true
			} else {
				hasNeedsInput = true
			}
		}
		switch {
		case hasNeedsInput:
			finding.Class = doctorNeedsInput
			finding.Action = "set each missing external secret with 'baha app secret set NAME --stdin'"
		case hasGeneratedMissing:
			finding.Class = doctorAutoFixable
			finding.Action = "generate declared application secrets through 'baha app apply'"
		default:
			finding.Class = doctorNeedsInput
			finding.Action = "inspect required-secret status and provide any missing value explicitly"
		}
	case "OpenBao application scope":
		finding.Class = doctorAutoFixable
		finding.Action = "reconverge the application OpenBao scope through 'baha app apply'"
	case "OpenBao control-plane runtime":
		finding.Class = doctorNeedsInput
		finding.Action = "repair the BaseHarbor control plane first with 'baha doctor' or operator-held OpenBao recovery material"
	case "manifest", "supported desired services", "manifest permissions", "workload discovery", "runtime permissions", "runtime orchestration", "runtime configuration":
		finding.Class = doctorManualAction
	}
	return finding
}

func findingsContain(findings []appDoctorFinding, name string) bool {
	for _, finding := range findings {
		if finding.Name == name {
			return true
		}
	}
	return false
}

func findingsNeedControlPlaneRepair(findings []appDoctorFinding) bool {
	for _, finding := range findings {
		switch finding.Name {
		case "OpenBao application scope", "application runtime broker", "required application secrets":
			return true
		}
	}
	return false
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
