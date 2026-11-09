// Package onboardingwizard implements the guided onboarding flow for
// r27-onboarding-wizard (t-20261018-r27-onboarding-wizard): one new
// user/organization is brought step by step to a functioning BaseHarbor
// installation with the minimum possible number of human decisions.
//
// The wizard is a presentation controller only. It performs no persistence and
// no mutation: every step reads through the injected discovery authority, and
// the effect happens once in the caller's commit step, after review.
// Back/cancel/EOF never mutate. The wizard never asks for information
// discovery already produced, never re-prompts an answered question, and keeps
// advanced authentication/trust details out of the default path. Bootstrap and
// attach are distinct intents; joining an existing installation never installs
// duplicate services; a valid reusable connection continues silently.
package onboardingwizard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Mode is the wizard entry mode. Bootstrap and attach are distinct intents
// (core-777: onboarding is separate from bootstrap); import joins with one
// received artifact; remove is local-only connection cleanup.
type Mode string

const (
	// ModeBootstrap creates a fresh local installation.
	ModeBootstrap Mode = "bootstrap"
	// ModeAttach joins an existing installation from this workstation.
	ModeAttach Mode = "attach"
	// ModeImport joins by importing one trusted device bundle or one
	// non-secret team profile.
	ModeImport Mode = "import"
	// ModeRemove removes only this workstation's local connection. Remote
	// revocation is a distinct explicit action owned by device-lifecycle;
	// local cleanup never deletes or revokes the installation.
	ModeRemove Mode = "remove"
	// ModeOwner configures the first Core operator and tenant through OIDC.
	ModeOwner Mode = "owner"
	// ModeInvite exports a protected one-time invite from an existing, local
	// Core. It is not an operator login or application bootstrap.
	ModeInvite Mode = "invite"
)

// Valid reports whether the mode is one of the distinct intents.
func (m Mode) Valid() bool {
	switch m {
	case ModeBootstrap, ModeAttach, ModeImport, ModeRemove, ModeOwner, ModeInvite:
		return true
	}
	return false
}

// AppIntent distinguishes attaching an existing application from registering
// a new one. The choice is explicit; it is never guessed.
type AppIntent string

const (
	// AppExisting attaches the application already registered at the
	// installation; joining installs no duplicate services.
	AppExisting AppIntent = "existing"
	// AppNew registers a new application explicitly.
	AppNew AppIntent = "new"
)

// Persistence is the credential persistence choice. The normal choice is no
// more complex than secure-for-future-use versus session-only.
type Persistence string

const (
	// PersistenceSessionOnly keeps material in the process or in the
	// protected per-user runtime directory only: never in Git, portable
	// intent or OpenBao.
	PersistenceSessionOnly Persistence = "session-only"
	// PersistenceSecure stores eligible material in the authoritative
	// protected store for reuse without repeated prompts.
	PersistenceSecure Persistence = "secure"
)

var (
	// ErrCancel reports an explicit cancel answer; nothing mutates.
	ErrCancel = errors.New("onboarding cancelled without changes")
	// ErrInvalidChoice reports a choice outside the offered set.
	ErrInvalidChoice = errors.New("onboarding choice is invalid")
	// ErrConflict reports an import or registration that conflicts with
	// current state; conflicts fail closed.
	ErrConflict = errors.New("onboarding conflicts with existing state")
	// ErrIncompleteConfiguration reports a broken decision combination; the
	// wizard fails closed before any effect.
	ErrIncompleteConfiguration = errors.New("onboarding configuration is incomplete")
)

// InstallState reports the facts discovery produced before the first question.
// Every field here is information the wizard is forbidden to ask for.
type InstallState struct {
	// HasInstallation reports whether a genuine Core installation is already
	// present on this workstation.
	HasInstallation bool
	// InstallationID is the installation identity when present.
	InstallationID string
	// Targets lists the declared Target names (may be empty).
	Targets []string
	// TrustedLocal reports whether a valid reusable management connection
	// already exists; when it does, no login and no credential question is
	// asked.
	TrustedLocal bool
	// ExistingAppFound reports that the application of the current checkout
	// is already registered at the installation.
	ExistingAppFound bool
	// ForcedAttach reports an explicit attach entry point (for example a
	// preselected Target argument). A forced attach still runs its
	// decisions even when trusted-local access exists; a repeat attach
	// without this flag continues silently.
	ForcedAttach bool
}

// Decision is one validated wizard outcome. It carries no mutation hook: the
// caller executes its commit step exactly once, after review approval.
type Decision struct {
	Mode         Mode
	App          AppIntent
	Target       string
	Persistence  Persistence
	ImportBundle string
	ImportFile   string
	TenantSlug   string
	TenantName   string
}

// Validate enforces the decision invariants independently of the controller.
func (d Decision) Validate() error {
	if !d.Mode.Valid() {
		return fmt.Errorf("%w: unknown mode %q", ErrInvalidChoice, d.Mode)
	}
	switch d.Mode {
	case ModeInvite:
		if strings.TrimSpace(d.Target) == "" {
			return fmt.Errorf("%w: invitation needs the installed local Core Target", ErrIncompleteConfiguration)
		}
	case ModeOwner:
		if strings.TrimSpace(d.TenantSlug) == "" || strings.TrimSpace(d.TenantName) == "" {
			return fmt.Errorf("%w: first-owner tenant name and slug are required", ErrIncompleteConfiguration)
		}
	case ModeBootstrap:
		if d.App != AppExisting && d.App != AppNew {
			return fmt.Errorf("%w: bootstrap needs an explicit existing-or-new application", ErrInvalidChoice)
		}
		if d.Persistence != PersistenceSessionOnly && d.Persistence != PersistenceSecure {
			return fmt.Errorf("%w: bootstrap needs the persistence choice", ErrInvalidChoice)
		}
	case ModeAttach:
		if strings.TrimSpace(d.Target) == "" {
			return fmt.Errorf("%w: attach needs the explicit Target", ErrIncompleteConfiguration)
		}
		if d.Persistence != "" && d.Persistence != PersistenceSessionOnly && d.Persistence != PersistenceSecure {
			return fmt.Errorf("%w: unknown persistence %q", ErrInvalidChoice, d.Persistence)
		}
	case ModeImport:
		if d.ImportBundle == "" && d.ImportFile == "" {
			return fmt.Errorf("%w: import needs exactly one received artifact", ErrIncompleteConfiguration)
		}
		if d.ImportBundle != "" && d.ImportFile != "" {
			return fmt.Errorf("%w: import takes exactly one of a device bundle or a team profile", ErrConflict)
		}
	case ModeRemove:
		if strings.TrimSpace(d.Target) == "" {
			return fmt.Errorf("%w: local removal needs the explicit Target", ErrIncompleteConfiguration)
		}
	}
	return nil
}

// Input carries the reader, writer, an optional preselected Target and the
// discovery authority. A nil Discover is treated as an empty state. The
// wizard never reads configuration, the filesystem or the network itself.
type Input struct {
	In     io.Reader
	Out    io.Writer
	Target string
	// Forced marks an explicit attach request (for example baha onboard
	// --target X). A forced attach runs its decisions even when a trusted
	// connection exists elsewhere; a repeat continues silently.
	Forced   bool
	Discover func(context.Context) (InstallState, error)
}

// Run executes the guided flow and returns the validated decision. It
// performs no mutation; the caller commits once after review.
func Run(ctx context.Context, in Input) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	if in.Out == nil {
		return Decision{}, ErrIncompleteConfiguration
	}
	discover := in.Discover
	if discover == nil {
		discover = func(context.Context) (InstallState, error) { return InstallState{}, nil }
	}
	state, err := discover(ctx)
	if err != nil {
		return Decision{}, err
	}
	target := strings.TrimSpace(in.Target)
	reader := bufio.NewReader(in.In)
	if state.TrustedLocal && !in.Forced {
		// Repeat operation while valid reusable access exists: continue
		// silently, zero questions, zero wizard-side mutation (UX rules
		// 2 and 4).
		return Decision{Mode: ModeAttach, Target: firstTarget(target, state)}, nil
	}
	if in.Forced {
		state.ForcedAttach = true
	}
	dec, err := askIntent(ctx, reader, in.Out, state, target)
	if err != nil {
		return Decision{}, err
	}
	if (dec.Mode == ModeAttach || dec.Mode == ModeRemove) && dec.Target == "" {
		dec.Target = firstTarget(target, state)
	}
	if err := dec.Validate(); err != nil {
		return Decision{}, err
	}
	fmt.Fprintln(in.Out, "Review:")
	for _, line := range reviewLines(dec) {
		fmt.Fprintf(in.Out, "  %s\n", line)
	}
	ok, err := askYesNo(ctx, reader, in.Out, "Apply this plan?", true)
	if err != nil {
		return Decision{}, err
	}
	if !ok {
		return Decision{}, ErrCancel
	}
	return dec, nil
}

func firstTarget(explicit string, state InstallState) string {
	if explicit != "" {
		return explicit
	}
	if len(state.Targets) == 1 {
		return state.Targets[0]
	}
	return ""
}

// askIntent walks the minimum question set for the current state.
func askIntent(ctx context.Context, reader *bufio.Reader, out io.Writer, state InstallState, preselected string) (Decision, error) {
	entries := map[string]string{"attach": "connect this workstation to an existing installation"}
	def := "attach"
	if !state.HasInstallation {
		entries["new"] = "create a new BaseHarbor installation here"
		def = "new"
	}
	if state.HasInstallation {
		entries["invite"] = "create a one-use workstation connection file"
		entries["owner"] = "initialize or verify Core operator and tenant with OIDC"
		def = "invite"
	}
	entries["import"] = "join with a device bundle or team profile you received"
	entries["remove"] = "remove only this workstation's local connection (nothing remote changes)"
	fmt.Fprintln(out, "What do you want to do?")
	choice, err := askChoice(ctx, reader, out, entries, def)
	if err != nil {
		return Decision{}, err
	}
	switch choice {
	case "invite":
		target, err := askTarget(ctx, reader, out, state, preselected)
		if err != nil {
			return Decision{}, err
		}
		return Decision{Mode: ModeInvite, Target: target}, nil
	case "owner":
		return askOwnerTenant(ctx, reader, out)
	case "new":
		return askBootstrap(ctx, reader, out, state)
	case "import":
		return askImport(ctx, reader, out)
	case "remove":
		target, err := askTarget(ctx, reader, out, state, preselected)
		if err != nil {
			return Decision{}, err
		}
		fmt.Fprintln(out, "Local cleanup revokes nothing remotely. Device revocation is a separate explicit action.")
		return Decision{Mode: ModeRemove, Target: target}, nil
	}
	// attach
	target, err := askTarget(ctx, reader, out, state, preselected)
	if err != nil {
		return Decision{}, err
	}
	dec := Decision{Mode: ModeAttach, Target: target}
	if state.TrustedLocal && !state.ForcedAttach {
		fmt.Fprintln(out, "Existing trusted access is valid; no login and no credential questions are needed.")
		return dec, nil
	}
	// Attaching is an explicit existing-versus-new decision unless the
	// registration was already discovered; joining never installs
	// duplicate services.
	app, err := askApplication(ctx, reader, out, state)
	if err != nil {
		return Decision{}, err
	}
	dec.App = app
	persist, err := askPersistence(ctx, reader, out)
	if err != nil {
		return Decision{}, err
	}
	dec.Persistence = persist
	return dec, nil
}

// askApplication asks the explicit existing-versus-new decision unless the
// registration was already discovered (then it is never asked). Claiming an
// existing application that discovery cannot find fails closed: joining must
// never install duplicate services nor guess an identity.
func askApplication(ctx context.Context, reader *bufio.Reader, out io.Writer, state InstallState) (AppIntent, error) {
	if state.ExistingAppFound {
		fmt.Fprintln(out, "This repository's application is already registered here; attaching it installs no duplicate services.")
		return AppExisting, nil
	}
	value, err := askChoice(ctx, reader, out, map[string]string{
		string(AppExisting): "attach the existing application (no duplicate services)",
		string(AppNew):      "register a new application",
	}, string(AppNew))
	if err != nil {
		return "", err
	}
	if value == string(AppExisting) {
		return "", fmt.Errorf("%w: no existing application was discovered for this checkout; register a new one", ErrConflict)
	}
	return AppNew, nil
}

// askBootstrap asks only what discovery could not decide: the application
// intent (when no registration was discovered) and the one persistence
// choice.
func askBootstrap(ctx context.Context, reader *bufio.Reader, out io.Writer, state InstallState) (Decision, error) {
	app := string(AppNew)
	if state.ExistingAppFound {
		app = string(AppExisting)
		fmt.Fprintln(out, "This repository's application is already registered here; attaching it installs no duplicate services.")
	} else if state.HasInstallation {
		// Attaching to an existing installation: existing versus new is a
		// real decision and asked explicitly.
		var err error
		app, err = askChoice(ctx, reader, out, map[string]string{
			string(AppExisting): "attach the existing application (no duplicate services)",
			string(AppNew):      "register a new application",
		}, string(AppNew))
		if err != nil {
			return Decision{}, err
		}
	}
	persist, err := askPersistence(ctx, reader, out)
	if err != nil {
		return Decision{}, err
	}
	decision := Decision{Mode: ModeBootstrap, App: AppIntent(app), Persistence: persist}
	if !state.HasInstallation && decision.App == AppNew {
		owner, err := askOwnerTenant(ctx, reader, out)
		if err != nil {
			return Decision{}, err
		}
		decision.TenantSlug, decision.TenantName = owner.TenantSlug, owner.TenantName
	}
	return decision, nil
}

// Owner setup is one default choice or two optional name fields; the actual
// OIDC provider setup stays in the existing owner-only commit authority.
func askOwnerTenant(ctx context.Context, reader *bufio.Reader, out io.Writer) (Decision, error) {
	choice, err := askChoice(ctx, reader, out, map[string]string{
		"default": "recommended baseharbor tenant; UUID generated automatically",
		"custom":  "choose tenant slug and display name",
	}, "default")
	if err != nil {
		return Decision{}, err
	}
	decision := Decision{Mode: ModeOwner, TenantSlug: "baseharbor", TenantName: "BaseHarbor"}
	if choice == "custom" {
		decision.TenantSlug, err = ask(ctx, reader, out, "Tenant slug", decision.TenantSlug)
		if err != nil {
			return Decision{}, err
		}
		decision.TenantName, err = ask(ctx, reader, out, "Tenant display name", decision.TenantName)
		if err != nil {
			return Decision{}, err
		}
	}
	return decision, nil
}

func askImport(ctx context.Context, reader *bufio.Reader, out io.Writer) (Decision, error) {
	kind, err := askChoice(ctx, reader, out, map[string]string{
		"bundle":  "one trusted device enrollment bundle (sensitive input)",
		"profile": "one non-secret team connection profile",
	}, "bundle")
	if err != nil {
		return Decision{}, err
	}
	dec := Decision{Mode: ModeImport}
	if kind == "bundle" {
		fmt.Fprintln(out, "The bundle stays untrusted until its import boundary validated it; no secret values are displayed.")
		dec.ImportBundle, err = ask(ctx, reader, out, "Bundle file", "")
	} else {
		dec.ImportFile, err = ask(ctx, reader, out, "Profile file", "")
	}
	if err != nil {
		return Decision{}, err
	}
	if dec.ImportBundle == "" && dec.ImportFile == "" {
		return Decision{}, fmt.Errorf("%w: an import needs a file path", ErrIncompleteConfiguration)
	}
	return dec, nil
}

func askTarget(ctx context.Context, reader *bufio.Reader, out io.Writer, state InstallState, preselected string) (string, error) {
	if preselected != "" {
		return preselected, nil
	}
	names := append([]string(nil), state.Targets...)
	sort.Strings(names)
	switch len(names) {
	case 0:
		return "", fmt.Errorf("%w: no Target is declared; declare or select one first", ErrIncompleteConfiguration)
	case 1:
		fmt.Fprintf(out, "Using the only declared Target %q (not asked: already discoverable).\n", names[0])
		return names[0], nil
	}
	fmt.Fprintln(out, "Select the installation Target:")
	for i, name := range names {
		fmt.Fprintf(out, "  %d  %s\n", i+1, name)
	}
	for {
		value, err := ask(ctx, reader, out, "Choice (number, name, or cancel)", "")
		if err != nil {
			return "", err
		}
		if isCancel(value) {
			return "", ErrCancel
		}
		if resolved, ok := resolveChoice(names, value); ok {
			return resolved, nil
		}
		fmt.Fprintln(out, "Invalid selection. Choose a displayed name or number, or cancel.")
	}
}

func askPersistence(ctx context.Context, reader *bufio.Reader, out io.Writer) (Persistence, error) {
	value, err := askChoice(ctx, reader, out, map[string]string{
		"yes": "store securely for future use",
		"no":  "keep this session only",
	}, "no")
	if err != nil {
		return "", err
	}
	if value == "yes" {
		return PersistenceSecure, nil
	}
	return PersistenceSessionOnly, nil
}

func reviewLines(d Decision) []string {
	lines := []string{"mode:    " + string(d.Mode)}
	if d.Target != "" {
		lines = append(lines, "target:  "+d.Target)
	}
	if (d.Mode == ModeOwner || d.Mode == ModeBootstrap) && d.TenantSlug != "" {
		lines = append(lines, "tenant:  "+d.TenantSlug+" ("+d.TenantName+")", "identity: verified operator OIDC / PKCE")
	}
	if d.App != "" {
		lines = append(lines, "app:     "+string(d.App))
	}
	if d.Persistence != "" {
		lines = append(lines, "store:   "+string(d.Persistence))
	}
	if d.ImportBundle != "" {
		lines = append(lines, "import:  device bundle "+d.ImportBundle)
	}
	if d.ImportFile != "" {
		lines = append(lines, "import:  team profile "+d.ImportFile)
	}
	return lines
}

// ask reads one line; empty input keeps the default; EOF propagates so the
// caller aborts without mutation.
func ask(ctx context.Context, reader *bufio.Reader, out io.Writer, label, def string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	prompt := label
	if def != "" {
		prompt += " [" + def + "]"
	}
	fmt.Fprintf(out, "%s: ", prompt)
	line, err := reader.ReadString('\n')
	trimmed := strings.TrimSpace(line)
	if err != nil {
		if trimmed == "" {
			return "", err
		}
		err = nil
	}
	if trimmed == "" {
		return def, nil
	}
	return trimmed, nil
}

func isCancel(value string) bool {
	switch strings.ToLower(value) {
	case "cancel", "q", "quit":
		return true
	}
	return false
}

// askChoice renders exactly one decision with its options and default.
// "cancel" always aborts.
func askChoice(ctx context.Context, reader *bufio.Reader, out io.Writer, allowed map[string]string, def string) (string, error) {
	keys := make([]string, 0, len(allowed))
	for key := range allowed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		marker := " "
		if key == def {
			marker = ">"
		}
		fmt.Fprintf(out, "  %s %s - %s\n", marker, key, allowed[key])
	}
	for {
		value, err := ask(ctx, reader, out, "Choice", def)
		if err != nil {
			return "", err
		}
		if isCancel(value) {
			return "", ErrCancel
		}
		value = strings.ToLower(value)
		if _, ok := allowed[value]; ok {
			return value, nil
		}
		fmt.Fprintln(out, "Choose one of the listed values.")
	}
}

func askYesNo(ctx context.Context, reader *bufio.Reader, out io.Writer, label string, def bool) (bool, error) {
	value, err := ask(ctx, reader, out, label+" [Y/n]", map[bool]string{true: "yes", false: "no"}[def])
	if err != nil {
		return false, err
	}
	switch strings.ToLower(value) {
	case "", "y", "yes", "j", "ja":
		return true, nil
	case "n", "no", "nein":
		return false, nil
	case "cancel", "q", "quit":
		return false, ErrCancel
	default:
		return false, fmt.Errorf("%w: expected yes or no, got %q", ErrInvalidChoice, value)
	}
}

func resolveChoice(names []string, value string) (string, bool) {
	for i, name := range names {
		if value == name || value == fmt.Sprintf("%d", i+1) {
			return name, true
		}
	}
	return "", false
}
