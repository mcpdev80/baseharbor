package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

// maintenanceClientHome is the CLIENT half of the two-home fixture: an owner-only
// directory that is provably a different filesystem root from the authority data
// root a resolved deployment's state actually lives under.
func maintenanceClientHome(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

// maintenancePasswordFile writes an owner-only client-local secret reference.
func maintenancePasswordFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "recovery-password")
	if err := os.WriteFile(path, []byte("maintenance-window-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// maintenanceDeployment is the single-deployment form of the fixture: the
// deployment is installed in the caller's current configuration and the working
// directory is left in its repository checkout.
func maintenanceDeployment(t *testing.T) resolvedApplication {
	t.Helper()
	resolved, _ := maintenanceInstalledDeployment(t)
	return resolved
}

// maintenanceInstalledDeployment provisions an installed application through the
// real repository resolution and deployment-recording seams and returns what
// resolveApplication yields for it plus its repository checkout. Its authority
// state lives under a temp data root, never under a client home.
//
// The repository checkout is deliberately NOT the current working directory: a
// registered-name resolution is the remote/MCP client shape, and staying outside
// the repository keeps the selected Target explicit.
func maintenanceInstalledDeployment(t *testing.T) (resolvedApplication, string) {
	t.Helper()
	configureTestTarget(t)
	t.Setenv("BASEHARBOR_TARGET", "")
	repo := t.TempDir()
	t.Chdir(repo)
	manifest := application.New(maintenanceAppName(t), "dev", true, false, false)
	mustWriteInitializedAuthoredManifest(t, application.RepositoryManifestName, manifest)
	resolved, err := resolveApplication(context.Background(), application.DefaultStore(), nil, "app maintenance")
	if err != nil {
		t.Fatal(err)
	}
	if err := recordAppliedDeployment(context.Background(), resolved, application.RuntimeFiles{}); err != nil {
		t.Fatal(err)
	}
	if _, err := deployment.LoadDeploymentRecord(resolved.DeploymentIdentity); err != nil {
		t.Fatalf("the fixture has no authority record: %v", err)
	}
	return resolved, repo
}

func mustWriteAuthoredManifestAt(t *testing.T, path string, m application.Manifest, store application.Store) {
	t.Helper()
	if err := os.WriteFile(path, []byte(m.AuthoredYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Sync(m); err != nil {
		t.Fatal(err)
	}
}

// maintenanceAppName keeps each test's application slug distinct, lowercase and
// bounded so one test's deployment state can never satisfy another's assertion.
// Subtest names contribute only their alphabetic prefix, so the slug stays valid.
func maintenanceAppName(t *testing.T) string {
	t.Helper()
	name := strings.TrimPrefix(t.Name(), "TestApplicationMaintenance")
	var kept strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			kept.WriteRune(r)
			continue
		}
		break
	}
	suffix := strings.ToLower(kept.String())
	if suffix == "" {
		suffix = "window"
	}
	return "maintenance-" + suffix
}

// maintenanceRemoteDeployment installs a Connector-backed deployment the way the
// production remote path does: the execution Target is the enrolled node and its
// authoritative state lives under that Target's root.
func maintenanceRemoteDeployment(t *testing.T, core, node deployment.ResolvedTarget) resolvedApplication {
	t.Helper()
	stateRoot, err := deployment.TargetStateRoot(node.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := application.New("remote-maintenance", "dev", true, false, false)
	store := application.Store{Root: stateRoot}
	if _, err := store.Create(manifest); err != nil {
		t.Fatal(err)
	}
	reloaded, manifestPath, err := store.Load(manifest.Name)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := deployment.NewDeploymentIdentity(node.Name, reloaded.ApplicationID, reloaded.Name, reloaded.Environment)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		Target: node, DeploymentIdentity: identity, Manifest: reloaded,
		ManifestPath: manifestPath, RepositoryRoot: filepath.Dir(manifestPath),
		TargetStateRoot: stateRoot, SourceAvailable: true,
	}
	deploymentRoot, err := deployment.DeploymentRoot(identity)
	if err != nil {
		t.Fatal(err)
	}
	resolved.DeploymentStateRoot = deploymentRoot
	resolved.Store = application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: node.Name}
	intent, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: resolved.RepositoryRoot, Manifest: manifestPath},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: node.RuntimeProvider},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(resolved.Store.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.Store.Sync(reloaded); err != nil {
		t.Fatal(err)
	}
	if !hasRemoteApplicationTarget(resolved) {
		t.Fatal("fixture Target is not remote")
	}
	_ = core
	return resolved
}

func maintenanceWindowPath(resolved resolvedApplication) string {
	return filepath.Join(resolved.stateRoot(), application.MaintenanceAuthorityFileName)
}

// maintenanceWindowRecord builds a durable window record for an authority view.
// A record whose revision is zero re-affirms the revision the window has already
// reached; a positive revision is the one the caller claims for a new operation.
func maintenanceWindowRecord(authority application.MaintenanceAuthority, operation, archive string, revision int) application.MaintenanceAuthorityRecord {
	if revision == 0 {
		revision = authority.Revision
	}
	return application.MaintenanceAuthorityRecord{
		ContractVersion: application.MaintenanceAuthorityContractVersion,
		ApplicationID:   authority.ApplicationID,
		DeploymentID:    authority.DeploymentID,
		Target:          authority.Target,
		Environment:     authority.Environment,
		Operation:       operation,
		Revision:        revision,
		Digest: application.MaintenanceDigest(authority.Target, authority.Environment, operation,
			revision, authority.Candidate, archive),
		ArchiveBinding: archive,
	}
}

func TestApplicationMaintenanceBindsWindowToAuthorityRevision(t *testing.T) {
	clientHome := maintenanceClientHome(t)
	archive := filepath.Join(clientHome, "unit.bhbackup")
	if err := os.WriteFile(archive, []byte("first recovery unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := maintenanceDeployment(t)
	if resolved.stateRoot() == clientHome {
		t.Fatal("the two-home fixture collapsed the client and authority roots")
	}

	authority, err := applicationMaintenanceAuthority(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if authority.RecordExists {
		t.Fatal("an unsubmitted window reported a durable record")
	}
	if authority.ApplicationID != resolved.DeploymentIdentity.ApplicationID ||
		authority.DeploymentID != resolved.DeploymentIdentity.DeploymentID {
		t.Fatalf("the window does not bind the selected deployment: %+v", authority)
	}
	refreshed, err := applicationMaintenanceAuthority(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Digest("backup", "sha256:x") != authority.Digest("backup", "sha256:x") {
		t.Fatal("the same window produced two different bindings")
	}
	if authority.Digest("backup", "sha256:x") == authority.Digest("backup", "sha256:y") {
		t.Fatal("the window binding ignores the submitted archive")
	}
	if authority.Digest("backup", "") == authority.Digest("restore", "") {
		t.Fatal("the window binding does not identify the operation")
	}
	if authority.Digest("backup", "") == application.MaintenanceDigest(authority.Target, authority.Environment, "backup", 99, authority.Candidate, "") {
		t.Fatal("the window binding ignores the revision")
	}

	// A lost response resolves to the same durable operation, never a replay.
	if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "backup", maintenanceFileDigest(archive)); err != nil {
		t.Fatalf("first durable backup window: %v", err)
	}
	if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "backup", maintenanceFileDigest(archive)); !errors.Is(err, errMaintenanceOperationAlreadyRecorded) {
		t.Fatalf("a lost response became a second mutation: %v", err)
	}
	persisted, found, err := application.LoadMaintenanceAuthority(resolved.stateRoot())
	if err != nil || !found {
		t.Fatalf("the durable window was not recorded: %v", err)
	}
	if persisted.Operation != "backup" || persisted.Revision != 1 {
		t.Fatalf("the window recorded the wrong durable operation: %+v", persisted)
	}
	resolved2, err := applicationMaintenanceAuthority(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved2.RecordExists || resolved2.Revision != persisted.Revision {
		t.Fatalf("the resolved window does not bind the durable revision: %+v", resolved2)
	}

	// A claimed revision the durable record has never reached is refused: a lost
	// window cannot be silently extended, and an old binding can never be
	// committed over the retained window.
	if _, _, err := application.CommitMaintenanceAuthority(resolved.stateRoot(),
		maintenanceWindowRecord(resolved2, "restore", "sha256:other", persisted.Revision+5)); err == nil {
		t.Fatal("committing a claimed future revision succeeded")
	}
	stale := persisted
	stale.Digest = application.MaintenanceDigest(stale.Target, stale.Environment, "backup", 77, stale.ArchiveBinding, "")
	if _, _, err := application.CommitMaintenanceAuthority(resolved.stateRoot(), stale); err == nil {
		t.Fatal("committing a stale window binding succeeded")
	}
}

func TestApplicationMaintenanceRejectsMissingAuthorityWithoutLocalFallback(t *testing.T) {
	clientHome := maintenanceClientHome(t)
	resolved := maintenanceDeployment(t)
	root, err := deployment.DeploymentRoot(resolved.DeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	// Remove only the authority record: application state and the client's own
	// files remain, so a client-local path is the (wrong) alternative available.
	if err := os.Remove(filepath.Join(root, "deployment.json")); err != nil {
		t.Fatal(err)
	}
	// The password file lives in the repository-parent home so the guard can prove
	// it refuses the whole directory, not just a repository sibling.
	password := maintenancePasswordFile(t, filepath.Dir(clientHome))
	var failure *machine.Error
	if _, err := applicationMaintenanceAuthority(resolved); !errors.As(err, &failure) || failure.Code != machine.ErrorOwnershipAmbiguous {
		t.Fatalf("maintenance without an authority record was not an ownership failure: %v", err)
	}

	ownerCalls := 0
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(context.Context, string, resolvedApplication, []string, io.Writer, io.Writer) error {
		ownerCalls++
		t.Fatal("maintenance without an authority reached a lifecycle owner")
		return nil
	}
	archive := filepath.Join(clientHome, "orphan.bhbackup")
	if err := os.WriteFile(archive, []byte("recovery unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"backup", "restore", "update", "destroy"} {
		err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
			Name: resolved.Manifest.Name, Operation: operation, Artifact: archive,
			PasswordFile: password, Confirmed: true, NoBackup: true, Now: time.Unix(1700000000, 0).UTC(),
		}, io.Discard, io.Discard)
		if !errors.As(err, &failure) || failure.Code != machine.ErrorOwnershipAmbiguous {
			t.Fatalf("%s without an authority was not refused as ownership: %v", operation, err)
		}
	}
	if ownerCalls != 0 {
		t.Fatalf("refused maintenance reached the lifecycle owner %d times", ownerCalls)
	}
	if _, err := os.Stat(maintenanceWindowPath(resolved)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused submission wrote a maintenance window")
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatalf("the refused submission touched the client archive: %v", err)
	}
}

func TestApplicationMaintenanceSecretChannelStaysClientLocal(t *testing.T) {
	clientHome := maintenanceClientHome(t)
	password := maintenancePasswordFile(t, clientHome)
	previous := maintenanceSecretTransport
	previousStat := maintenanceReferenceStat
	t.Cleanup(func() {
		maintenanceSecretTransport = previous
		maintenanceReferenceStat = previousStat
	})

	var transportSaw string
	maintenanceSecretTransport = func(path string) ([]byte, error) {
		transportSaw = path
		return os.ReadFile(path)
	}
	secret, err := readMaintenanceSecretInput(password)
	if err != nil || len(secret) == 0 {
		t.Fatalf("safe channel read failed: %q %v", secret, err)
	}
	if transportSaw != password {
		t.Fatalf("the secret transport resolved a different path: %q", transportSaw)
	}
	if strings.ContainsRune(string(secret), '\n') {
		t.Fatal("the trailing newline survived the secret read")
	}
	zeroBytes(secret)

	// A filename must never reach the transport: the client cannot send a laptop
	// path to the authority and hope it resolves there.
	transportSaw = ""
	maintenanceSecretTransport = func(path string) ([]byte, error) {
		transportSaw = path
		t.Fatalf("the secret transport read a client filename as a path: %q", path)
		return nil, nil
	}
	var failure *machine.Error
	if _, err := readMaintenanceSecretInput("recovery-password"); !errors.As(err, &failure) || failure.Code != machine.ErrorValidationFailed {
		t.Fatalf("a bare laptop filename was not refused as a typed path error: %v", err)
	}
	if transportSaw != "" {
		t.Fatal("the rejected filename still reached the transport")
	}

	// A plaintext value in the reference position is rejected and never echoed.
	const plaintext = "hunter2-hunter2-hunter2"
	if _, err := readMaintenanceSecretInput(plaintext); err == nil {
		t.Fatal("a plaintext value was accepted as secret input")
	} else if strings.Contains(err.Error(), plaintext) {
		t.Fatalf("the secret value leaked into the error: %v", err)
	}

	short := filepath.Join(clientHome, "short-password")
	if err := os.WriteFile(short, []byte("tiny"), 0o600); err != nil {
		t.Fatal(err)
	}
	maintenanceSecretTransport = func(path string) ([]byte, error) { return os.ReadFile(path) }
	if _, err := readMaintenanceSecretInput(short); !errors.As(err, &failure) || failure.Code != machine.ErrorValidationFailed {
		t.Fatalf("short secret input was not typed: %v", err)
	}
	if _, err := requireClientLocalMaintenancePasswordFile(short); !errors.As(err, &failure) || failure.Code != machine.ErrorValidationFailed {
		t.Fatalf("the channel guard accepted a too-short password file: %v", err)
	}
	world := filepath.Join(clientHome, "world-readable")
	if err := os.WriteFile(world, []byte("maintenance-window-password"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := requireClientLocalMaintenancePasswordFile(world); err == nil {
		t.Fatal("a group-readable password file was accepted")
	}
	if _, err := requireClientLocalMaintenancePasswordFile(filepath.Join(clientHome, "absent")); err == nil {
		t.Fatal("an absent password file was accepted")
	}
	// A reference whose identity cannot be established is never treated as a
	// usable channel.
	maintenanceReferenceStat = func(string) (os.FileInfo, error) {
		return nil, errors.New("identity unavailable")
	}
	if _, err := requireClientLocalMaintenancePasswordFile(password); err == nil {
		t.Fatal("an unresolvable password file reference was accepted")
	}
	maintenanceReferenceStat = previousStat
	link := filepath.Join(clientHome, "link-to-password")
	if err := os.Symlink(password, link); err != nil {
		t.Skipf("the test host cannot create symlinks: %v", err)
	}
	if _, err := requireClientLocalMaintenancePasswordFile(link); err == nil {
		t.Fatal("a symlinked password file was accepted")
	}
}

func TestApplicationMaintenanceArchiveChannelRequiresTwoHomes(t *testing.T) {
	clientHome := maintenanceClientHome(t)
	authorityHome := maintenanceClientHome(t)
	archive := filepath.Join(clientHome, "unit.bhbackup")
	if err := os.WriteFile(archive, []byte("recovery unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	clean, err := requireClientLocalMaintenanceArtifact(archive)
	if err != nil || clean != archive {
		t.Fatalf("client-local archive rejected: %q %v", clean, err)
	}
	// Different local and server roots: a name that exists only in the authority
	// home is not the client's archive, and the client cannot offer it as one.
	serverOnly := filepath.Join(authorityHome, "server-side.bhbackup")
	if err := os.WriteFile(serverOnly, []byte("authority-only"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(archive); err != nil {
		t.Fatal(err)
	}
	// A spelling that reaches the same file only by walking out and back in is not a
	// canonical reference and is refused even though the target exists.
	if _, err := os.Stat(filepath.Join(filepath.Dir(clientHome), "walk.bhbackup")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the walk-up fixture is not clean")
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(clientHome), "walk.bhbackup"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []string{"", "unit.bhbackup", "./unit.bhbackup",
		filepath.Join(clientHome, "..", filepath.Base(clientHome), "walk.bhbackup"),
		filepath.Dir(archive)} {
		if _, err := requireClientLocalMaintenanceArtifact(unsafe); err == nil {
			t.Fatalf("unsafe archive reference accepted: %q", unsafe)
		}
	}
	// An empty file and a path that does not exist on the client are never recovery
	// units.
	empty := filepath.Join(clientHome, "empty.bhbackup")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := requireClientLocalMaintenanceArtifact(empty); err == nil {
		t.Fatal("an empty archive reference was accepted")
	}
	// A repository-internal path is refused so archived state cannot be committed.
	repo := maintenanceRepositoryFixture(t)
	if _, err := requireClientLocalMaintenanceArtifact(filepath.Join(repo, "absent-in-repo.bhbackup")); err == nil {
		t.Fatal("an archive reference inside a Git worktree was accepted")
	}
	if digest := maintenanceFileDigest(archive); !strings.HasPrefix(digest, "sha256:") || digest == "sha256:unreadable" {
		t.Fatalf("the archive digest is not a binding: %q", digest)
	}
	if maintenanceFileDigest(filepath.Join(clientHome, "gone")) != "sha256:unreadable" {
		t.Fatal("an unreadable archive produced a binding anyway")
	}
	if maintenanceFileDigest("relative-name") != "sha256:invalid" {
		t.Fatal("a relative reference produced a durable binding")
	}
}

// maintenanceRepositoryFixture makes the test binary's own checkout the current
// working directory so the worktree guard is exercised on a real repository.
func maintenanceRepositoryFixture(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(wd)
	for dir := wd; ; {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("the maintenance test does not run inside a BaseHarbor worktree")
		}
		dir = parent
	}
}

func TestApplicationMaintenanceRemoteRoutingRejectsUnsupportedNativeMaintenance(t *testing.T) {
	ctx, core, node := remoteApplicationCoreFixture(t)
	ctx = withCoreAuthority(ctx, core)
	stateRoot, err := deployment.TargetStateRoot(node.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := application.New("remote-maintenance", "dev", true, false, false)
	store := application.Store{Root: stateRoot}
	if _, err := store.Create(manifest); err != nil {
		t.Fatal(err)
	}
	reloaded, manifestPath, err := store.Load(manifest.Name)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := deployment.NewDeploymentIdentity(node.Name, reloaded.ApplicationID, reloaded.Name, reloaded.Environment)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedApplication{
		Target: node, DeploymentIdentity: identity, Manifest: reloaded,
		ManifestPath: manifestPath, RepositoryRoot: filepath.Dir(manifestPath),
		TargetStateRoot: stateRoot,
		Store:           application.Store{Root: filepath.Join(stateRoot, "state"), Namespace: node.Name},
		SourceAvailable: true,
	}
	deploymentRoot, err := deployment.DeploymentRoot(identity)
	if err != nil {
		t.Fatal(err)
	}
	resolved.DeploymentStateRoot = deploymentRoot
	resolved.Store = application.Store{Root: filepath.Join(deploymentRoot, "state"), Namespace: node.Name}
	intent, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployment.SaveDeploymentRecord(deployment.DeploymentRecord{
		Version:  deployment.DeploymentRecordVersion,
		Identity: identity,
		Source:   deployment.DeploymentSource{Kind: "repository", Repository: resolved.RepositoryRoot, Manifest: manifestPath},
		Applied:  deployment.AppliedDeployment{Intent: intent, RuntimeProvider: node.RuntimeProvider},
	}); err != nil {
		t.Fatal(err)
	}
	if !hasRemoteApplicationTarget(resolved) {
		t.Fatal("fixture Target is not remote")
	}
	if err := os.MkdirAll(resolved.Store.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := resolved.Store.Sync(reloaded); err != nil {
		t.Fatal(err)
	}
	transport, scope, err := remoteApplicationTransport(ctx, resolved, &applicationBindingTransport{})
	if err != nil || transport == nil || scope.Runtime == "" {
		t.Fatalf("remote transport fixture: %v %v", transport, err)
	}
	if _, err := transport.LiveCapabilities(scope); err != nil {
		t.Fatalf("the retained remote transport cannot observe capabilities: %v", err)
	}
	// The submission uses its own recording transport, so the fixture's capability
	// observation above is not counted: an unsupported intent must leave the
	// connector it submits through completely untouched.
	base := &applicationBindingTransport{}
	ownerCalls := 0
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(context.Context, string, resolvedApplication, []string, io.Writer, io.Writer) error {
		ownerCalls++
		t.Fatal("unsupported remote maintenance reached a lifecycle owner")
		return nil
	}
	for _, operation := range []string{"backup", "restore", "update"} {
		t.Run(operation, func(t *testing.T) {
			err := submitApplicationMaintenance(ctx, resolved, applicationMaintenanceOptions{
				Name: resolved.Manifest.Name, Operation: operation, Confirmed: true, NoBackup: true,
			}, io.Discard, io.Discard)
			var failure *machine.Error
			if !errors.As(err, &failure) || failure.Code != machine.ErrorCapabilityMissing {
				t.Fatalf("unsupported native %s maintenance was not rejected explicitly: %v", operation, err)
			}
			if !maintenanceRejectedBeforeTransport(err) {
				t.Fatal("the rejection was not distinguishable from a transport failure")
			}
			if base.capabilities != 0 || base.dispatches != 0 {
				t.Fatalf("unsupported maintenance reached the connector: %d capability reads, %d dispatches",
					base.capabilities, base.dispatches)
			}
			if _, err := os.Stat(maintenanceWindowPath(resolved)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("rejected maintenance wrote a durable window")
			}
		})
	}
	if ownerCalls != 0 {
		t.Fatalf("rejected maintenance reached the lifecycle owner %d times", ownerCalls)
	}
	// Capability discovery also refuses an unsafe client reference before any
	// transport: the authority never resolves a workstation path.
	if err := submitApplicationMaintenance(ctx, resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Artifact: "leftover-archive.bhbackup", Confirmed: true,
	}, io.Discard, io.Discard); err == nil {
		t.Fatal("remote maintenance accepted a laptop filename as an archive channel")
	}
	if base.capabilities != 0 || base.dispatches != 0 {
		t.Fatalf("an unsafe reference reached the connector: %d capability reads, %d dispatches",
			base.capabilities, base.dispatches)
	}
	// The accepted set is exactly the domain contract; routing cannot widen it, and
	// routing alone is never proof of remote support.
	if strings.Join(maintenanceOperations, ",") != strings.Join(application.MaintenanceOperations, ",") {
		t.Fatal("the client operation set drifted from the domain contract")
	}
}

func TestApplicationMaintenanceTypedDenialCancelAndLostResponse(t *testing.T) {
	clientHome := maintenanceClientHome(t)
	archive := filepath.Join(clientHome, "window.bhbackup")
	if err := os.WriteFile(archive, []byte("recovery unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved := maintenanceDeployment(t)

	var routed []string
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(_ context.Context, operation string, got resolvedApplication, args []string, _, _ io.Writer) error {
		routed = append(routed, operation+"@"+got.Target.Name+" "+strings.Join(args, " "))
		return nil
	}

	// Denial: destructive intent without explicit approval is typed, never prompted.
	var failure *machine.Error
	err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Now: time.Unix(1700000000, 0).UTC(),
	}, io.Discard, io.Discard)
	if !errors.As(err, &failure) || failure.Code != machine.ErrorApprovalRequired || failure.CauseCode != "destructive_approval_required" {
		t.Fatalf("unapproved destroy was not a typed denial: %v", err)
	}
	if len(routed) != 0 {
		t.Fatalf("a denied submission reached the lifecycle owner: %v", routed)
	}

	// Cancel: an already cancelled submission stops before any owner or window.
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err := submitApplicationMaintenance(cancelled, resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true,
	}, io.Discard, io.Discard); err == nil {
		t.Fatal("a cancelled maintenance submission proceeded")
	} else if !errors.As(err, &failure) || failure.Code != machine.ErrorConflict {
		t.Fatalf("cancellation was not typed as a conflict: %v", err)
	}
	if _, found, err := application.LoadMaintenanceAuthority(resolved.stateRoot()); err != nil || found {
		t.Fatalf("a denied or cancelled submission persisted a window: %v found=%t", err, found)
	}
	if len(routed) != 0 {
		t.Fatalf("a cancelled submission reached the lifecycle owner: %v", routed)
	}

	// The routed submission carries the exact approval scope the owner needs, and
	// the window records it exactly once.
	if err := submitApplicationMaintenance(context.Background(), resolved, applicationMaintenanceOptions{
		Name: resolved.Manifest.Name, Operation: "destroy", Confirmed: true, FullReset: true,
	}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(routed) != 1 || !strings.Contains(routed[0], "--yes --full-reset") {
		t.Fatalf("destroy lost its approval scope: %v", routed)
	}
	authority, err := applicationMaintenanceAuthority(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !authority.RecordExists || authority.Revision != 1 {
		t.Fatalf("the routed operation recorded no durable window: %+v", authority)
	}
	// The routed operation's own binding is durable and idempotent.
	if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "destroy", ""); !errors.Is(err, errMaintenanceOperationAlreadyRecorded) {
		t.Fatalf("a lost response became a second mutation: %v", err)
	}
	// A claimed revision the window has never reached is refused.
	if _, _, err := application.CommitMaintenanceAuthority(resolved.stateRoot(),
		maintenanceWindowRecord(authority, "restore", "", 4)); err == nil {
		t.Fatal("a claimed future revision was committed")
	}
}

func TestApplicationMaintenanceRejectsUnsafeInputAndUnknownOperation(t *testing.T) {
	if _, err := parseApplicationMaintenanceArgs([]string{"app"}); err == nil {
		t.Fatal("a maintenance submission without --operation was accepted")
	} else {
		var usage *cli.UsageError
		if !errors.As(err, &usage) {
			t.Fatalf("missing operation was not a usage error: %v", err)
		}
	}
	for _, args := range [][]string{
		{"--operation", "replay", "app"},
		{"--operation"},
		{"--operation", "backup", "--artifact"},
		{"--operation", "backup", "--password-file"},
		{"--operation", "backup", "app", "second"},
		{"--operation", "backup", "--bogus"},
	} {
		if _, err := parseApplicationMaintenanceArgs(args); err == nil {
			t.Fatalf("malformed maintenance arguments accepted: %v", args)
		}
	}
	options, err := parseApplicationMaintenanceArgs([]string{"app", "--operation", "restore", "--artifact", "/tmp/a.bhbackup", "--password-file", "/tmp/pw", "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Name != "app" || options.Operation != "restore" || options.Artifact != "/tmp/a.bhbackup" ||
		options.PasswordFile != "/tmp/pw" || !options.Confirmed {
		t.Fatalf("maintenance options parsed incorrectly: %+v", options)
	}
	// An unsupported operation is typed even when a caller bypasses the parser, and
	// it stops before the authority lookup.
	var failure *machine.Error
	err = submitApplicationMaintenance(context.Background(), resolvedApplication{
		Target:   deployment.ResolvedTarget{Name: "local", AccessProvider: "local"},
		Manifest: application.New("typed", "dev", true, false, false),
	}, applicationMaintenanceOptions{Name: "typed", Operation: "replay"}, io.Discard, io.Discard)
	if !errors.As(err, &failure) || failure.Code != machine.ErrorValidationFailed {
		t.Fatalf("unknown operation was not typed: %v", err)
	}
}

func TestApplicationMaintenanceWindowRejectsReplacedArtifact(t *testing.T) {
	clientHome := maintenanceClientHome(t)
	archive := filepath.Join(clientHome, "rotating.bhbackup")
	resolved := maintenanceDeployment(t)
	previous := maintenanceLifecycleOwner
	t.Cleanup(func() { maintenanceLifecycleOwner = previous })
	maintenanceLifecycleOwner = func(context.Context, string, resolvedApplication, []string, io.Writer, io.Writer) error { return nil }
	if err := os.WriteFile(archive, []byte("first-unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "backup", maintenanceFileDigest(archive)); err != nil {
		t.Fatal(err)
	}
	// Re-submitting the same operation against a replaced archive is a different
	// durable submission, never a silent no-op.
	if err := os.WriteFile(archive, []byte("second-unit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "backup", maintenanceFileDigest(archive)); err != nil {
		t.Fatalf("a replaced archive was refused instead of recorded: %v", err)
	}
	persisted, found, err := application.LoadMaintenanceAuthority(resolved.stateRoot())
	if err != nil || !found {
		t.Fatalf("the replacement was not durable: %v", err)
	}
	if persisted.Operation != "backup" || persisted.Revision != 2 {
		t.Fatalf("the window did not record the replacement: %+v", persisted)
	}
	// An unreadable archive reference binds nothing, so it can never be recorded as
	// a completed recovery point.
	if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "backup", maintenanceFileDigest(filepath.Join(clientHome, "gone"))); err == nil {
		t.Fatal("an unreadable archive bound a maintenance window")
	}
	// A corrupt protected record is never silently replaced.
	if err := os.WriteFile(maintenanceWindowPath(resolved), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := applicationMaintenanceAuthority(resolved); err == nil {
		t.Fatal("a corrupt maintenance window was accepted")
	}
}

func TestApplicationMaintenanceWindowCannotBeAdoptedByAnotherDeployment(t *testing.T) {
	resolved, _ := maintenanceInstalledDeployment(t)
	if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "backup", "sha256:unit"); err != nil {
		t.Fatal(err)
	}
	other, otherRepo := maintenanceInstalledDeployment(t)
	otherAuthority, err := applicationMaintenanceAuthority(other)
	if err != nil {
		t.Fatal(err)
	}
	foreign := maintenanceWindowRecord(otherAuthority, "backup", "sha256:unit", 0)
	data, err := json.MarshalIndent(foreign, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// The client's own resolved view is what must refuse adoption: the retained
	// record is the only window it may bind, and this one names another deployment.
	if err := os.WriteFile(maintenanceWindowPath(resolved), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	var failure *machine.Error
	if _, err := applicationMaintenanceAuthority(resolved); !errors.As(err, &failure) || failure.Code != machine.ErrorOwnershipAmbiguous {
		t.Fatalf("a window from another deployment was adopted: %v", err)
	}
	if err := commitApplicationMaintenanceWindow(context.Background(), resolved, "destroy", ""); !errors.Is(err, application.ErrMaintenanceAuthorityForeign) && err == nil {
		t.Fatalf("a foreign window was accepted without refusal: %v", err)
	}
	// A retained window is only usable by the deployment that owns its identity, so
	// writing one deployment's window into another's protected state cannot succeed
	// either. Both homes must refuse it and neither may advance it.
	if err := os.WriteFile(maintenanceWindowPath(other), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := applicationMaintenanceAuthority(other); err == nil {
		t.Fatal("a window with the matching identity but a foreign record was accepted without validation")
	}
	if err := commitApplicationMaintenanceWindow(context.Background(), other, "destroy", ""); err == nil {
		t.Fatal("a submission advanced a window whose record was refused")
	}
	_ = otherRepo
}

func TestApplicationMaintenanceAuthorityRecordIsOwnerOnlyAndBounded(t *testing.T) {
	stateRoot := maintenanceClientHome(t)
	other := maintenanceClientHome(t)
	record := application.MaintenanceAuthorityRecord{
		ContractVersion: application.MaintenanceAuthorityContractVersion,
		ApplicationID:   "11111111-1111-4111-8111-111111111111",
		DeploymentID:    "22222222-2222-4222-8222-222222222222",
		Target:          "managed", Environment: "dev", Operation: "backup",
		Revision: 1, Digest: application.MaintenanceDigest("managed", "dev", "backup", 1, "", ""),
	}
	if _, _, err := application.CommitMaintenanceAuthority(stateRoot, record); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(stateRoot, application.MaintenanceAuthorityFileName))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("maintenance window is not owner-only: %o", info.Mode().Perm())
	}
	// A group-accessible state directory is never written to.
	for _, root := range []string{other, "relative/state"} {
		if root == other {
			if err := os.Chmod(other, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if _, isNew, err := application.CommitMaintenanceAuthority(root, record); err == nil || isNew {
			t.Fatalf("maintenance window was written to %q: %v", root, err)
		}
	}
	// Unknown operations and malformed bindings fail closed.
	broken := record
	broken.Operation = "replay"
	if _, isNew, err := application.CommitMaintenanceAuthority(stateRoot, broken); err == nil || isNew {
		t.Fatalf("an unknown maintenance operation reached durable state: %v", err)
	}
	broken = record
	broken.Digest = "sha256:short"
	if _, isNew, err := application.CommitMaintenanceAuthority(stateRoot, broken); err == nil || isNew {
		t.Fatalf("a truncated binding reached durable state: %v", err)
	}
	broken = record
	broken.ContractVersion = "baseharbor.maintenance-authority/v9"
	if _, isNew, err := application.CommitMaintenanceAuthority(stateRoot, broken); err == nil || isNew {
		t.Fatalf("an unsupported contract reached durable state: %v", err)
	}
	// The durable window never carries secret material or a server path.
	data, err := os.ReadFile(filepath.Join(stateRoot, application.MaintenanceAuthorityFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password", "hunter2", "token", stateRoot} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatalf("the maintenance window recorded %q: %s", forbidden, data)
		}
	}
}
