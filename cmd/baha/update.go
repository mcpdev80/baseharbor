package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/coreupdate"
 bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const defaultReleaseAPIBase = "https://api.github.com/repos/mcpdev80/baseharbor"

var (
	releaseAPIBase    = defaultReleaseAPIBase
	releaseHTTPClient = &http.Client{Timeout: 15 * time.Second}
)

type baseHarborRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Digest             string `json:"digest"`
	} `json:"assets"`
}

type selfUpdateCheck struct {
	Installed          string                  `json:"installed"`
	Channel            string                  `json:"channel"`
	Target             string                  `json:"target"`
	ReleaseURL         string                  `json:"release_url"`
	AssetName          string                  `json:"asset_name"`
	AssetURL           string                  `json:"asset_url"`
	AssetDigest        string                  `json:"asset_digest"`
	ChecksumsURL       string                  `json:"checksums_url"`
	Relation           string                  `json:"relation"`
	Platform           string                  `json:"platform"`
	Prerelease         bool                    `json:"prerelease"`
	CoreReconciliation string                  `json:"core_reconciliation"`
	CoreExpected       []coreupdate.Desired    `json:"core_expected,omitempty"`
	CoreBacking        []coreupdate.BackingPin `json:"core_backing,omitempty"`
	CoreInstallationID string                  `json:"core_installation_id,omitempty"`
	CoreInstallReady   bool                    `json:"core_install_ready,omitempty"`
	CoreInstallPhase   string                  `json:"core_install_phase,omitempty"`
}

type selfUpdateOptions struct {
	Check   bool
	Yes     bool
	Channel string
	Version string
}

func updateCommand() *cli.Command {
	return &cli.Command{
		Name:    "update",
		Summary: "Safely inspect or update BaseHarbor itself",
		Usage:   "baha update [--check] [--yes] [--channel stable|rc | --version VERSION]",
		Long:    "Checks or installs published BaseHarbor releases. Stable is the default channel; prereleases are considered only when --channel rc or an explicit prerelease --version is supplied. Mutation requires --yes, verifies release checksums and the candidate binary before replacement, retains a recovery binary, and verifies the updated CLI/runtime before reporting success.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "update")
			if err != nil {
				return err
			}
			args = filtered
			if format == outputJSON {
				args = append(args, "--check")
			}
			opts, err := parseSelfUpdateOptions(args)
			if err != nil {
				return err
			}
			check, err := inspectSelfUpdate(ctx, version, opts)
			if err != nil {
				return err
			}
			if opts.Check {
				if format == outputJSON {
					return writeJSON(out, check)
				}
				formatSelfUpdateCheck(out, check)
				return nil
			}
			if !opts.Yes {
				formatSelfUpdateCheck(out, check)
				return usageError("BaseHarbor self-update requires explicit confirmation", "Re-run with --yes after reviewing the selected release.")
			}
			return performSelfUpdate(ctx, check, opts, out, errOut)
		},
	}
}

func parseSelfUpdateOptions(args []string) (selfUpdateOptions, error) {
	opts := selfUpdateOptions{Channel: "stable"}
	channelSet := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check":
			opts.Check = true
		case "--yes", "-y":
			opts.Yes = true
		case "--channel":
			i++
			if i >= len(args) {
				return selfUpdateOptions{}, usageError("--channel requires stable or rc", "Run 'baha update --help' for usage.")
			}
			opts.Channel = strings.ToLower(strings.TrimSpace(args[i]))
			channelSet = true
			if opts.Channel != "stable" && opts.Channel != "rc" {
				return selfUpdateOptions{}, usageError("unsupported update channel: "+opts.Channel, "Use stable or rc.")
			}
		case "--version":
			i++
			if i >= len(args) || strings.TrimSpace(args[i]) == "" {
				return selfUpdateOptions{}, usageError("--version requires a release version", "Example: baha update --check --version 0.3.0")
			}
			opts.Version = normalizeReleaseVersion(args[i])
		default:
			return selfUpdateOptions{}, usageError("unknown baha update option: "+args[i], "Run 'baha update --help' for usage.")
		}
	}
	if opts.Version != "" && channelSet {
		return selfUpdateOptions{}, usageError("--version and --channel cannot be combined", "Choose an exact release or a release channel.")
	}
	if opts.Check && opts.Yes {
		return selfUpdateOptions{}, usageError("--check and --yes cannot be combined", "Use --check for a read-only inspection or --yes to perform the update.")
	}
	return opts, nil
}

func inspectSelfUpdate(ctx context.Context, installed string, opts selfUpdateOptions) (selfUpdateCheck, error) {
	release, err := resolveSelfUpdateRelease(ctx, opts)
	if err != nil {
		return selfUpdateCheck{}, err
	}
	target := normalizeReleaseVersion(release.TagName)
	if target == "" {
		return selfUpdateCheck{}, errors.New("selected release has no valid tag")
	}
	assetName := fmt.Sprintf("baseharbor_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var assetURL, assetDigest, checksumsURL string
	for _, asset := range release.Assets {
		switch asset.Name {
		case assetName:
			assetURL = asset.BrowserDownloadURL
			assetDigest = asset.Digest
		case "checksums.txt":
			checksumsURL = asset.BrowserDownloadURL
		}
	}
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		return selfUpdateCheck{}, fmt.Errorf("BaseHarbor self-update is not packaged for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if assetURL == "" {
		return selfUpdateCheck{}, fmt.Errorf("release %s does not contain required asset %s", release.TagName, assetName)
	}
	if checksumsURL == "" {
		return selfUpdateCheck{}, fmt.Errorf("release %s does not contain checksums.txt", release.TagName)
	}
	installedNormalized := normalizeReleaseVersion(installed)
	relation := "unknown"
	if installedNormalized == "" || installedNormalized == "dev" || strings.HasPrefix(installedNormalized, "dev-") {
		relation = "development-build"
	} else if cmp, ok := compareReleaseVersions(installedNormalized, target); ok {
		switch {
		case cmp < 0:
			relation = "update-available"
		case cmp == 0:
			relation = "up-to-date"
		default:
			relation = "target-older"
		}
	}
	coreReconciliation := "not_required"
	var coreExpected []coreupdate.Desired
	var coreBacking []coreupdate.BackingPin
	var coreState coreinstallation.State
	if _, installedCore := existingControlPlaneForSelfUpdate(ctx); installedCore {
		coreReconciliation = "unavailable"
		targetInfo, targetErr := effectiveTarget(ctx)
		if targetErr == nil {
			root, rootErr := targetRuntimeStateRoot(targetInfo)
			if rootErr == nil {
				state, stateErr := coreinstallation.Load(root)
				if stateErr == nil {
					coreState = state
				}
			}
		}
		if manifest, err := coreupdate.LoadRelease(target); err == nil {
			coreExpected, coreBacking = manifest.Providers, manifest.Backing
		} else {
			coreReconciliation = "unavailable_unpinned"
		}
	}
	return selfUpdateCheck{
		CoreReconciliation: coreReconciliation,
		CoreInstallationID: coreState.ID,
		CoreInstallReady:   coreState.Ready,
		CoreInstallPhase:   coreState.Phase,
		CoreExpected:       coreExpected,
		CoreBacking:        coreBacking,
		Installed:          installedNormalized,
		Channel:            opts.Channel,
		Target:             target,
		ReleaseURL:         release.HTMLURL,
		AssetName:          assetName,
		AssetURL:           assetURL,
		AssetDigest:        assetDigest,
		ChecksumsURL:       checksumsURL,
		Relation:           relation,
		Platform:           runtime.GOOS + "/" + runtime.GOARCH,
		Prerelease:         release.Prerelease,
	}, nil
}

func resolveSelfUpdateRelease(ctx context.Context, opts selfUpdateOptions) (baseHarborRelease, error) {
	if opts.Version != "" {
		var release baseHarborRelease
		if err := fetchReleaseJSON(ctx, releaseAPIBase+"/releases/tags/v"+opts.Version, &release); err != nil {
			return baseHarborRelease{}, fmt.Errorf("resolve BaseHarbor release v%s: %w", opts.Version, err)
		}
		if release.Draft {
			return baseHarborRelease{}, fmt.Errorf("release v%s is still a draft", opts.Version)
		}
		return release, nil
	}
	if opts.Channel == "stable" {
		var release baseHarborRelease
		if err := fetchReleaseJSON(ctx, releaseAPIBase+"/releases/latest", &release); err != nil {
			return baseHarborRelease{}, fmt.Errorf("resolve latest stable BaseHarbor release: %w", err)
		}
		if release.Draft || release.Prerelease {
			return baseHarborRelease{}, errors.New("latest stable release endpoint returned a draft or prerelease")
		}
		return release, nil
	}
	var releases []baseHarborRelease
	if err := fetchReleaseJSON(ctx, releaseAPIBase+"/releases?per_page=30", &releases); err != nil {
		return baseHarborRelease{}, fmt.Errorf("resolve BaseHarbor rc releases: %w", err)
	}
	for _, release := range releases {
		if !release.Draft && release.Prerelease {
			return release, nil
		}
	}
	return baseHarborRelease{}, errors.New("no published BaseHarbor prerelease is available")
}

func fetchReleaseJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "baseharbor-baha/"+normalizeReleaseVersion(version))
	resp, err := releaseHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("release API returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode release API response: %w", err)
	}
	return nil
}

func formatSelfUpdateCheck(out io.Writer, check selfUpdateCheck) {
	installed := check.Installed
	if installed == "" {
		installed = "unknown"
	}
	fmt.Fprintf(out, "Installed version: %s\n", installed)
	fmt.Fprintf(out, "Channel: %s\n", check.Channel)
	fmt.Fprintf(out, "Available version: %s\n", check.Target)
	fmt.Fprintf(out, "Platform: %s\n", check.Platform)
	if check.CoreInstallationID != "" {
		fmt.Fprintf(out, "Core installation: %s (phase: %s, ready: %t)\n", check.CoreInstallationID, check.CoreInstallPhase, check.CoreInstallReady)
	}
	if check.CoreReconciliation == "unavailable_unpinned" {
		fmt.Fprintln(out, "Core provider upgrade: unavailable; no release-owned immutable provider set is installed for this target")
	} else if check.CoreReconciliation == "unavailable" {
		fmt.Fprintln(out, "Core provider upgrade: unavailable; installed SQL/Secrets/Identity must not be upgraded by binary-only update")
	} else {
		fmt.Fprintln(out, "Core provider upgrade: not required for current installation")
	}
	fmt.Fprintf(out, "Release asset: %s\n", check.AssetName)
	if check.Prerelease {
		fmt.Fprintln(out, "Release type: prerelease (explicitly selected)")
	} else {
		fmt.Fprintln(out, "Release type: stable")
	}
	switch check.Relation {
	case "up-to-date":
		fmt.Fprintln(out, "Update: up to date")
	case "update-available":
		fmt.Fprintln(out, "Update: available")
	case "target-older":
		fmt.Fprintln(out, "Update: selected target is older than the installed version")
	case "development-build":
		fmt.Fprintln(out, "Update: installed build is a development build; release ordering is not inferred")
	default:
		fmt.Fprintln(out, "Update: version ordering could not be determined")
	}
	fmt.Fprintln(out, "Checksum manifest: available")
	fmt.Fprintln(out, "No changes were made.")
}

func normalizeReleaseVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	return value
}

func compareReleaseVersions(left, right string) (int, bool) {
	l, ok := parseReleaseVersion(left)
	if !ok {
		return 0, false
	}
	r, ok := parseReleaseVersion(right)
	if !ok {
		return 0, false
	}
	for i := 0; i < 3; i++ {
		if l.numbers[i] < r.numbers[i] {
			return -1, true
		}
		if l.numbers[i] > r.numbers[i] {
			return 1, true
		}
	}
	if l.prerelease == r.prerelease {
		return 0, true
	}
	if l.prerelease == "" {
		return 1, true
	}
	if r.prerelease == "" {
		return -1, true
	}
	return comparePrerelease(l.prerelease, r.prerelease), true
}

type parsedReleaseVersion struct {
	numbers    [3]int
	prerelease string
}

func parseReleaseVersion(value string) (parsedReleaseVersion, bool) {
	value = normalizeReleaseVersion(value)
	mainPart, prerelease, _ := strings.Cut(value, "-")
	parts := strings.Split(mainPart, ".")
	if len(parts) != 3 {
		return parsedReleaseVersion{}, false
	}
	var parsed parsedReleaseVersion
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return parsedReleaseVersion{}, false
		}
		parsed.numbers[i] = n
	}
	parsed.prerelease = prerelease
	return parsed, true
}

func comparePrerelease(left, right string) int {
	lparts := strings.Split(left, ".")
	rparts := strings.Split(right, ".")
	limit := len(lparts)
	if len(rparts) < limit {
		limit = len(rparts)
	}
	for i := 0; i < limit; i++ {
		if lparts[i] == rparts[i] {
			continue
		}
		ln, lerr := strconv.Atoi(lparts[i])
		rn, rerr := strconv.Atoi(rparts[i])
		switch {
		case lerr == nil && rerr == nil:
			if ln < rn {
				return -1
			}
			return 1
		case lerr == nil:
			return -1
		case rerr == nil:
			return 1
		case lparts[i] < rparts[i]:
			return -1
		default:
			return 1
		}
	}
	if len(lparts) < len(rparts) {
		return -1
	}
	if len(lparts) > len(rparts) {
		return 1
	}
	return 0
}

func inspectCoreRuntimePlan(ctx context.Context, targetVersion string, state coreinstallation.State, runtimeProvider bhruntime.RuntimeProvider) (coreupdate.Plan, error) {
 catalog, err := coreupdate.LoadRelease(targetVersion)
 if err != nil { return coreupdate.Plan{}, err }
 if !state.Ready || state.ID == "" { return coreupdate.Plan{}, errors.New("Core installation is not ready for provider inventory") }
 services := []struct{kind coreupdate.ProviderKind; project, service string}{
  {coreupdate.SQL, bhruntime.SharedProjectName(state.Spec.Target), "postgres-member-1"},
  {coreupdate.Secrets, bhruntime.SharedProjectName(state.Spec.Target), "openbao-member-1"},
  {coreupdate.Identity, bhruntime.SharedProjectName(state.Spec.Target + "-core"), "keycloak-1"},
 }
 var existing []coreupdate.Realization
 for _, item := range services {
  id, err := runtimeProvider.ProjectServiceImageIdentity(ctx,item.project,item.service)
  if err != nil { return coreupdate.Plan{}, fmt.Errorf("inspect Core %s runtime image: %w",item.kind,err) }
  ref := strings.TrimSpace(id.Reference)
  pos := strings.LastIndex(ref,":")
  if pos <= strings.LastIndex(ref,"/") { return coreupdate.Plan{},fmt.Errorf("Core %s runtime version is not explicit",item.kind) }
  v := ref[pos+1:]
  if item.kind == coreupdate.SQL { v = strings.SplitN(v,"-",2)[0] }
  digest := strings.TrimSpace(id.Digest)
  if at := strings.Index(digest,"@sha256:"); at >= 0 { digest = digest[at+1:] }
  existing = append(existing,coreupdate.Realization{Kind:item.kind,Installation:state.ID,Scope:"shared",Instance:item.service,Owner:"baseharbor",Image:ref,Digest:digest,Version:v})
 }
 return coreupdate.Build(targetVersion, existing, catalog.Providers)
}
