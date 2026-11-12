package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/database"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

func workstationCommand() *cli.Command {
	return &cli.Command{
		Name:    "workstation",
		Summary: "Invite, list and revoke developer workstations connected to this Core",
		Usage:   "baha workstation new | baha workstation list | baha workstation revoke NAME [--yes]",
		Long:    "Run on the Core host. New adds a device for an existing user. List shows each enrolled workstation, its owner and certificate expiry. Revoke disables that workstation's current certificate and immediately verifies the result.",
		Children: []*cli.Command{
			{
				Name:    "new",
				Summary: "Issue a one-use invitation for another workstation owned by an existing user",
				Usage:   "baha workstation new [--user LOGIN] [--name DEVICE] [--certificate-ttl 5y] [--output PRIVATE_FILE]",
				Run:     workstationNewCommand,
			},
			{
				Name:    "list",
				Summary: "List enrolled workstations and certificate expiry",
				Usage:   "baha workstation list",
				Run:     workstationListCommand,
			},
			{
				Name:    "revoke",
				Summary: "Revoke a workstation's current certificate",
				Usage:   "baha workstation revoke NAME [--yes]",
				Run:     workstationRevokeCommand,
			},
		},
	}
}

type workstationAuthority struct {
	target    deployment.ResolvedTarget
	store     *database.WorkstationEnrollmentStore
	authority *ws.Authority
	issuer    *platformopenbao.ServiceIssuer
	close     func()
}

func openWorkstationAuthority(ctx context.Context) (*workstationAuthority, error) {
	target, err := effectiveTarget(ctx)
	if err != nil {
		return nil, err
	}
	files, err := initialOperatorLocalCoreFiles(ctx, target)
	if err != nil {
		return nil, errors.New("workstation administration must run on the locally owned Core host")
	}
	if strings.TrimSpace(target.TenantID) == "" {
		return nil, errors.New("the selected Core Target has no installation tenant; run baha onboard on the Core host first")
	}
	check, cancel := context.WithTimeout(ctx, 45*time.Second)
	provider, files, err := openBaoRuntime(check)
	if err != nil {
		cancel()
		return nil, errors.New("managed local Core Secrets authority is unavailable")
	}
	status, err := platformopenbao.Inspect(check, provider, files)
	if err != nil || !status.Initialized || status.Sealed || platformopenbao.CheckManager(check, provider, files) != nil {
		cancel()
		return nil, errors.New("managed local OpenBao must be initialized, unsealed and owned to administer workstations")
	}
	issuer := platformopenbao.NewServiceIssuer(provider, files)
	dsn, err := locallyOwnedCorePostgresDSN(files)
	if err != nil {
		cancel()
		return nil, err
	}
	pool, err := database.Open(check, database.Config{DSN: dsn, ConnectTimeout: 10 * time.Second})
	if err != nil {
		cancel()
		return nil, errors.New("local Core workstation registry is unavailable")
	}
	store := database.NewWorkstationEnrollmentStore(pool)
	authority, err := ws.New(store, issuer, store)
	if err != nil {
		pool.Close()
		cancel()
		return nil, err
	}
	return &workstationAuthority{target: target, store: store, authority: authority, issuer: issuer, close: func() { pool.Close(); cancel() }}, nil
}

func workstationListCommand(ctx context.Context, args []string, out, _ io.Writer) error {
	if len(args) != 0 {
		return usageError("baha workstation list does not accept arguments", "Run baha workstation list.")
	}
	owner, err := openWorkstationAuthority(ctx)
	if err != nil {
		return err
	}
	defer owner.close()
	facts, err := owner.store.ListRegistryFacts(ctx, owner.target.TenantID)
	if err != nil {
		return errors.New("could not read the workstation registry from the selected Core")
	}
	fmt.Fprintf(out, "Workstations · %s\n", owner.target.Name)
	if len(facts) == 0 {
		fmt.Fprintln(out, "  No workstations enrolled.")
		return nil
	}
	now := time.Now()
	fmt.Fprintf(out, "%-24s %-20s %-20s %-20s %s\n", "NAME", "ISSUED", "EXPIRES", "USER", "STATUS / REMAINING")
	for _, fact := range facts {
		scope := ws.Scope{TenantID: owner.target.TenantID, Station: fact.Station}
		revoked, stateErr := owner.authority.RevokedState(ctx, scope)
		state := "active · " + remainingLifetime(fact.ExpiresAt.Sub(now))
		if stateErr != nil {
			state = "unknown · revocation state unavailable"
		} else if revoked {
			state = "revoked"
		} else if !fact.ExpiresAt.After(now) {
			state = "expired"
		}
		ownerLabel := fact.OwnerLogin
		if ownerLabel == "" {
			ownerLabel = "(unassigned)"
		}
		fmt.Fprintf(out, "%-24s %-20s %-20s %-20s %s\n", fact.Station, fact.IssuedAt.Local().Format("2006-01-02 15:04"), fact.ExpiresAt.Local().Format("2006-01-02 15:04"), ownerLabel, state)
	}
	return nil
}

func workstationNewCommand(ctx context.Context, args []string, out, _ io.Writer) error {
	var login, station, outputFile string
	ttl := ws.MaxCertificateTTL
	ttlSet := false
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return usageError("workstation new option requires a value", "Run baha workstation new --help.")
		}
		value := args[i+1]
		switch args[i] {
		case "--user":
			login = value
		case "--name", "--station":
			station = value
		case "--output":
			outputFile = value
		case "--certificate-ttl":
			parsed, err := ws.ParseCertificateTTL(value)
			if err != nil {
				return usageError("invalid certificate lifetime", "Choose a duration up to 5y, such as 5y or 90d.")
			}
			ttl = parsed
			ttlSet = true
		default:
			return unknownOptionUsage("baha workstation new", args[i], "--user", "--name", "--station", "--output", "--certificate-ttl")
		}
		i++
	}
	owner, err := openWorkstationAuthority(ctx)
	if err != nil {
		return err
	}
	defer owner.close()
	users, _, err := owner.store.WorkstationUsers(ctx, owner.target.TenantID)
	if err != nil {
		return errors.New("could not read the installation user directory")
	}
	var active []database.WorkstationUser
	for _, user := range users {
		if user.Status == "active" {
			active = append(active, user)
		}
	}
	if login == "" {
		if noInput(ctx) || !readerIsTerminal(onboardingInput) {
			return errors.New("choose an existing user with --user LOGIN; see baha user list")
		}
		if len(active) == 0 {
			return errors.New("there are no active users; create the first user with baha onboard --invite")
		}
		fmt.Fprintln(out, "Choose the person who owns this workstation:")
		for i, user := range active {
			fmt.Fprintf(out, "  %d. %s (%s)\n", i+1, user.DisplayName, user.Login)
		}
		fmt.Fprint(out, "Choice: ")
		line, readErr := bufio.NewReader(onboardingInput).ReadString('\n')
		if readErr != nil && line == "" {
			return errors.New("user selection was not provided")
		}
		var index int
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d", &index); err != nil || index < 1 || index > len(active) {
			return errors.New("choose a listed user number")
		}
		login = active[index-1].Login
	}
	user, ok, err := owner.store.WorkstationUserByLogin(ctx, owner.target.TenantID, login)
	if err != nil || !ok || user.Status != "active" {
		return fmt.Errorf("active user %q was not found; run baha user list", login)
	}
	if station == "" {
		if noInput(ctx) || !readerIsTerminal(onboardingInput) {
			return errors.New("workstation name is required in non-interactive mode; use --name DEVICE")
		}
		fmt.Fprint(out, "Workstation name: ")
		line, readErr := bufio.NewReader(onboardingInput).ReadString('\n')
		if readErr != nil && line == "" {
			return errors.New("workstation name was not provided")
		}
		station = strings.TrimSpace(line)
	}
	if !ttlSet && !noInput(ctx) && readerIsTerminal(onboardingInput) {
		fmt.Fprint(out, "Workstation certificate lifetime [5y]: ")
		line, readErr := bufio.NewReader(onboardingInput).ReadString('\n')
		if readErr != nil && line == "" {
			return errors.New("certificate lifetime was not provided")
		}
		value := strings.TrimSpace(line)
		if value != "" {
			parsed, err := ws.ParseCertificateTTL(value)
			if err != nil {
				return errors.New("certificate lifetime must be no greater than 5y, for example 5y or 90d")
			}
			ttl = parsed
		}
	}
	if err := (ws.Scope{TenantID: owner.target.TenantID, Station: station}).Validate(); err != nil {
		return errors.New("invalid workstation name; use letters, numbers, hyphens or underscores")
	}
	if outputFile == "" {
		root, err := deployment.DataRoot()
		if err != nil {
			return err
		}
		dir := filepath.Join(root, "onboarding-invitations")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		outputFile = filepath.Join(dir, "baseharbor-onboard-"+station+".json")
	}
	coreOrigin, err := loadOwnedCoreLANEndpoint(owner.target.Name)
	if err != nil {
		return fmt.Errorf("managed Core HTTPS endpoint unavailable: %w", err)
	}
	if err := issueLocalWorkstationInvitation(withTargetOverride(ctx, owner.target.Name), owner.target, station, coreOrigin, outputFile, ttl, login, nil); err != nil {
		return err
	}
	fmt.Fprintf(out, "Invitation created for %s (%s): %s\nTransfer this private file to the new workstation.\n", user.DisplayName, login, outputFile)
	return nil
}

func workstationRevokeCommand(ctx context.Context, args []string, out, errOut io.Writer) error {
	name := ""
	yes := false
	for _, arg := range args {
		if arg == "--yes" {
			yes = true
			continue
		}
		if strings.HasPrefix(arg, "-") || name != "" {
			return usageError("expected one workstation name and optional --yes", "Run baha workstation revoke NAME [--yes].")
		}
		name = arg
	}
	if name == "" {
		return usageError("workstation name is required", "Run baha workstation list, then baha workstation revoke NAME.")
	}
	owner, err := openWorkstationAuthority(ctx)
	if err != nil {
		return err
	}
	defer owner.close()
	scope := ws.Scope{TenantID: owner.target.TenantID, Station: name}
	if err := scope.Validate(); err != nil {
		return usageError("invalid workstation name", "Names use letters, numbers, hyphens or underscores.")
	}
	if _, _, _, _, _, ok, readErr := owner.store.ReadRegistryFact(ctx, scope); readErr != nil || !ok {
		return fmt.Errorf("workstation %q is not registered on the selected Core", name)
	}
	revoked, err := owner.authority.RevokedState(ctx, scope)
	if err != nil {
		return errors.New("cannot verify the workstation certificate status; refusing to change it")
	}
	if revoked {
		fmt.Fprintf(out, "Workstation %s is already revoked.\n", name)
		return nil
	}
	if !yes {
		if noInput(ctx) {
			return errors.New("workstation revocation requires confirmation; rerun with --yes")
		}
		fmt.Fprintf(out, "Revoke workstation %q from Core Target %q? Its current certificate will stop working. [y/N] ", name, owner.target.Name)
		line, readErr := bufio.NewReader(onboardingInput).ReadString('\n')
		if readErr != nil && len(line) == 0 {
			return errors.New("workstation revocation was not confirmed")
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Cancelled; no workstation was revoked.")
			return nil
		}
	}
	if err := owner.authority.Revoke(ctx, scope); err != nil {
		return fmt.Errorf("could not revoke workstation %q: %w", name, err)
	}
	verified, err := owner.authority.RevokedState(ctx, scope)
	if err != nil || !verified {
		return errors.New("revocation was requested but could not be verified; check baha workstation list before continuing")
	}
	fmt.Fprintf(out, "Workstation %s revoked and verified.\n", name)
	return nil
}

func remainingLifetime(remaining time.Duration) string {
	if remaining <= 0 {
		return "expired"
	}
	days := int(remaining.Hours() / 24)
	if days >= 365 {
		return fmt.Sprintf("%dy %dd left", days/365, days%365)
	}
	if days > 0 {
		return fmt.Sprintf("%dd left", days)
	}
	return "less than a day left"
}
