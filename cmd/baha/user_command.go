package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

func userCommand() *cli.Command {
	return &cli.Command{Name: "user", Summary: "Manage people and their enrolled workstations", Usage: "baha user list | baha user offboard LOGIN [--yes]", Children: []*cli.Command{
		{Name: "list", Summary: "List people and their enrolled workstation counts", Usage: "baha user list", Run: userListCommand},
		{Name: "offboard", Summary: "Disable a person and revoke all their workstations", Usage: "baha user offboard LOGIN [--yes]", Run: userOffboardCommand},
	}}
}

func userListCommand(ctx context.Context, args []string, out, _ io.Writer) error {
	if len(args) != 0 {
		return usageError("baha user list does not accept arguments", "Run baha user list.")
	}
	owner, err := openWorkstationAuthority(ctx)
	if err != nil {
		return err
	}
	defer owner.close()
	users, devices, err := owner.store.WorkstationUsers(ctx, owner.target.TenantID)
	if err != nil {
		return errors.New("could not read the installation user directory")
	}
	fmt.Fprintf(out, "Users · %s\n", owner.target.Name)
	if len(users) == 0 {
		fmt.Fprintln(out, "  No onboarded users.")
		return nil
	}
	fmt.Fprintf(out, "%-22s %-32s %-32s %-12s %s\n", "LOGIN", "NAME", "EMAIL", "STATUS", "DEVICES")
	for _, user := range users {
		fmt.Fprintf(out, "%-22s %-32s %-32s %-12s %d\n", user.Login, user.DisplayName, user.Email, user.Status, devices[user.Login])
	}
	return nil
}

func userOffboardCommand(ctx context.Context, args []string, out, _ io.Writer) error {
	login := ""
	yes := false
	for _, arg := range args {
		if arg == "--yes" {
			yes = true
			continue
		}
		if strings.HasPrefix(arg, "-") || login != "" {
			return usageError("expected one login and optional --yes", "Run baha user offboard LOGIN [--yes].")
		}
		login = arg
	}
	if login == "" {
		return usageError("user login is required", "Run baha user list, then baha user offboard LOGIN.")
	}
	owner, err := openWorkstationAuthority(ctx)
	if err != nil {
		return err
	}
	defer owner.close()
	user, found, err := owner.store.WorkstationUserByLogin(ctx, owner.target.TenantID, login)
	if err != nil || !found {
		return fmt.Errorf("user %q is not registered on this Core", login)
	}
	facts, err := owner.store.WorkstationOwnerStations(ctx, owner.target.TenantID, login)
	if err != nil {
		return errors.New("could not read the user's workstation inventory")
	}
	if !yes {
		if noInput(ctx) || !readerIsTerminal(onboardingInput) {
			return errors.New("user offboarding requires confirmation; rerun with --yes")
		}
		fmt.Fprintf(out, "Offboard %s (%s)? This disables the account and revokes %d workstation certificate(s). [y/N] ", user.DisplayName, login, len(facts))
		line, readErr := bufio.NewReader(onboardingInput).ReadString('\n')
		if readErr != nil && line == "" {
			return errors.New("offboarding was not confirmed")
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Cancelled; no changes were made.")
			return nil
		}
	}
	// Disable sign-in first. If a later certificate revoke fails, retrying is safe:
	// Keycloak remains disabled and the database still retains the active audit row.
	dataDir, err := deployment.TargetStateRoot(owner.target.Name)
	if err != nil {
		return err
	}
	if err := identityprovider.DisableManagedOperatorUser(ctx, dataDir, owner.target.Name, owner.target.Name, "dev", login); err != nil {
		return fmt.Errorf("could not disable Keycloak user %q; no workstation certificates were changed: %w", login, err)
	}
	// Mark the user inactive before enumerating devices. This transaction also
	// removes pending grants; grant creation and leaf persistence lock/check the
	// same user row, so no new device can race into the inventory after this point.
	if err := owner.store.OffboardWorkstationUser(ctx, owner.target.TenantID, login); err != nil {
		return fmt.Errorf("Keycloak account is disabled, but Core directory status could not be updated; retry offboarding: %w", err)
	}
	facts, err = owner.store.WorkstationOwnerStations(ctx, owner.target.TenantID, login)
	if err != nil {
		return errors.New("user is disabled, but the final workstation inventory could not be read; retry offboarding")
	}
	for _, fact := range facts {
		scope := ws.Scope{TenantID: owner.target.TenantID, Station: fact.Station}
		revoked, checkErr := owner.authority.RevokedState(ctx, scope)
		if checkErr != nil {
			return fmt.Errorf("user is disabled, but revocation state for workstation %q is unknown; retry offboarding", fact.Station)
		}
		if !revoked {
			if err := owner.authority.Revoke(ctx, scope); err != nil {
				return fmt.Errorf("user is disabled, but workstation %q could not be revoked; retry offboarding: %w", fact.Station, err)
			}
		}
		verified, verifyErr := owner.authority.RevokedState(ctx, scope)
		if verifyErr != nil || !verified {
			return fmt.Errorf("user is disabled, but workstation %q revocation could not be verified; retry offboarding", fact.Station)
		}
	}
	fmt.Fprintf(out, "User %s offboarded; account disabled and %d workstation certificate(s) revoked and verified.\n", login, len(facts))
	return nil
}
