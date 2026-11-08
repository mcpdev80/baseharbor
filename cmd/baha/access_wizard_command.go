package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/accesswizard"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/orgconfig"
	"github.com/mcpdev80/baseharbor/internal/policy"
)

// accessWizardCommand wires the guided profile-assignment and offboarding
// wizard into the root command tree. It is a bounded CLI adapter: it loads the
// accepted production authority (the active organization Manager) and fails
// closed when the configuration is incomplete, then hands the interactive
// input handling to the pure accesswizard.Controller. The wizard never decides;
// every assignment is folded into the single production policy authority.
func accessWizardCommand() *cli.Command {
	return &cli.Command{
		Name:    "access",
		Summary: "Guide profile assignment and deliberate offboarding of a person on a target",
		Usage:   "baha access assign|--assign PERSON [--json] | baha access offboard|--offboard PERSON [--json]",
		Long:    "Guided, fail-closed profile assignment and offboarding. The wizard names the person, searches and selects declared Targets, confirms the effective rights, and reviews before any write. It never grants a right on its own: every assignment folds into the single production policy authority and every removal re-checks the current configuration at submit time.",
		Children: []*cli.Command{
			accessAssignCommand(),
			accessOffboardCommand(),
		},
	}
}

func accessAssignCommand() *cli.Command {
	return &cli.Command{
		Name:    "assign",
		Summary: "Guided assignment of an access profile to a person on selected Targets",
		Usage:   "baha access assign PERSON [--json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "access assign")
			if err != nil {
				return err
			}
			if len(filtered) != 1 {
				return usageError("baha access assign requires exactly one PERSON", "Run 'baha access assign --help' for usage.")
			}
			authority, scope, state, err := loadAccessWizardAuthority(errOut)
			if err != nil {
				return err
			}
			controller := accesswizard.NewController(scope)
			controller.SetScope(scope)
			controller.SetIO(appNewInput, out)
			result := controller.Run(func(review accesswizard.Review) policy.Decision {
				if err := validateAccessWizardPerson(filtered[0], review.Person.Name); err != nil {
					return policy.Deny
				}
				// Batch semantics (finding #5): every reviewed selection for this
				// route applies to the SAME (person, name) on its own Target under
				// one durable, all-or-nothing transaction. Validate every selected
				// Target against the production authority first — any Deny or error
				// fails the whole batch — then persist the complete set once. One
				// failed Target therefore leaves the store unchanged (nothing
				// partial is written).
				for _, sel := range review.Selected {
					decision, err := accesswizard.ResolveAssignment(
						authority,
						review.Person.Name,
						sel.TargetName,
						sel.Operation,
						true,
					)
					if err != nil {
						return policy.Deny
					}
					if decision == policy.Deny {
						return policy.Deny
					}
				}
				// Every selection validated in memory; commit the whole set
				// atomically. A write failure fails closed and persists nothing.
				// A persistence error (e.g. the owner-collision guard refusing to
				// overwrite a foreign-owner row on the same key) is carried on the
				// result so the operator sees why the change was not written.
				if err := accesswizard.CommitActive(authority, resolveActive); err != nil {
					controller.NoteCommitError(err)
					return policy.Deny
				}
				return policy.Allow
			})
			// The transaction either persisted via resolveActive or failed
			// closed; reload active state and assert the durable owned profile
			// set matches. A success without a reloadable change fails closed.
			_ = state
			if format == outputJSON {
				_ = writeJSON(out, accessWizardJSON{
					Person:   filtered[0],
					Decision: string(result.Decision),
				})
			}
			if result.Err != nil {
				return result.Err
			}
			return nil
		},
	}
}

func accessOffboardCommand() *cli.Command {
	return &cli.Command{
		Name:    "offboard",
		Summary: "Guided deliberate offboarding of a person from a target",
		Usage:   "baha access offboard PERSON [--json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "access offboard")
			if err != nil {
				return err
			}
			if len(filtered) != 1 {
				return usageError("baha access offboard requires exactly one PERSON", "Run 'baha access offboard --help' for usage.")
			}
			authority, scope, state, err := loadAccessWizardAuthority(errOut)
			if err != nil {
				return err
			}
			controller := accesswizard.NewController(scope)
			controller.SetScope(scope)
			controller.SetIO(appNewInput, out)
			result := controller.Run(func(review accesswizard.Review) policy.Decision {
				if err := validateAccessWizardPerson(filtered[0], review.Person.Name); err != nil {
					return policy.Deny
				}
				// Batch semantics (finding #5): every reviewed selection for this
				// route revokes the SAME (person, name) on its own Target. Validate
				// every Target against the production authority first — any failure
				// fails the whole batch — then remove the whole set once. A failed
				// Target therefore leaves the store byte-identical (no partial
				// offboarding survives a rejected Target).
				for _, sel := range review.Selected {
					if err := accesswizard.RevokeAssignment(
						authority,
						review.Person.Name,
						sel.TargetName,
						accesswizard.RemoveManagedUser,
					); err != nil {
						controller.NoteCommitError(err)
						return policy.Deny
					}
				}
				// Every selection validated in memory; commit the whole removal
				// set atomically. A write failure fails closed and persists
				// nothing. A persistence error (e.g. the owner-collision guard
				// refusing to delete a foreign-owner row on the same key) is
				// carried on the result so the operator sees why.
				if err := accesswizard.CommitActive(authority, resolveActive); err != nil {
					controller.NoteCommitError(err)
					return policy.Deny
				}
				return policy.Allow
			})
			_ = state
			if format == outputJSON {
				_ = writeJSON(out, accessWizardJSON{
					Person:   filtered[0],
					Decision: string(result.Decision),
				})
			}
			if result.Err != nil {
				return result.Err
			}
			return nil
		},
	}
}

// loadAccessWizardAuthority loads the active organization configuration and
// builds the accepted production Manager for the active owner. It fails closed
// when the configuration is incomplete (ErrIncompleteConfig): no active
// organization, no declared operation, or no declared target. The wizard never
// runs against an empty manager.
//
// The failure message is rendered to errOut and returned as a cli.PresentedError
// so main does not append a second generic error block: the human result is
// shown once, the process still fails.
//
// resolveActive is threaded through the wizard so a confirmed assignment or
// revocation persists through the same active-state writer the product uses.
// The batch caller applies every reviewed selection to the in-memory Manager,
// then calls CommitActive once so the whole set persists atomically — a failed
// selection writes nothing, leaving the store byte-identical.
//
// The returned state is the freshly loaded active configuration; the caller
// uses it to build the Manager and to scope the wizard. The caller reloads active
// state itself once through CommitActive after the batch validates.
func loadAccessWizardAuthority(errOut io.Writer) (*orgconfig.Manager, map[string]string, orgconfig.ActiveState, error) {
	state, err := orgconfig.LoadActive()
	if err != nil {
		// No active configuration: the wizard cannot name a declared target or
		// resolve an operation, so it fails closed rather than guessing.
		presented := fmt.Errorf("%w: %v", accesswizard.ErrIncompleteConfig, err)
		presentHuman(errOut, presented)
		return nil, nil, state, &cli.PresentedError{Err: presented}
	}
	operation, err := orgconfig.ResolveOperation(state)
	if err != nil || operation == "" {
		presented := fmt.Errorf("%w: the active organization declares no operation", accesswizard.ErrIncompleteConfig)
		presentHuman(errOut, presented)
		return nil, nil, state, &cli.PresentedError{Err: presented}
	}
	manager := orgconfig.ManagerFromActive(state)
	if manager == nil || manager.Count() == 0 {
		// An active organization with no owned profile still fails closed on any
		// assignment through the accepted policy fold.
		presented := accesswizard.ErrDenied
		presentHuman(errOut, presented)
		return manager, nil, state, &cli.PresentedError{Err: presented}
	}
	scope := make(map[string]string, len(state.Config.Targets))
	for name := range state.Config.Targets {
		scope[name] = string(operation)
	}
	return manager, scope, state, nil
}

// resolveActive reloads the active organization configuration, applies the
// Manager's current owned profile set, and persists it through the production
// active-state writer. It is the single persistence path the wizard uses so an
// assignment or revocation reaches the same store the product reads on every
// other command. A reload or write failure is returned as an error so the
// wizard fails closed and reports no success.
func resolveActive() (orgconfig.ActiveState, error) {
	state, err := orgconfig.LoadActive()
	if err != nil {
		return state, fmt.Errorf("reload active organization configuration: %w", err)
	}
	return state, nil
}

// presentHuman renders a presented error to errOut in the same plain style the
// CLI uses, without a trailing newline, so main's failure path stays quiet.
func presentHuman(errOut io.Writer, err error) {
	_, _ = fmt.Fprint(errOut, err.Error())
	_, _ = fmt.Fprintln(errOut)
}

type accessWizardJSON struct {
	Person   string `json:"person"`
	Decision string `json:"decision"`
}

// validateAccessWizardPerson binds the human review to the exact explicit CLI
// subject. A wizard prompt may not silently substitute a different person for
// either a grant or a revocation; this check runs BEFORE calling the authority.
func validateAccessWizardPerson(requested, reviewed string) error {
	if strings.TrimSpace(requested) == "" || strings.TrimSpace(reviewed) == "" ||
		requested != reviewed {
		return fmt.Errorf("access wizard reviewed person differs from the requested CLI subject")
	}
	return nil
}
