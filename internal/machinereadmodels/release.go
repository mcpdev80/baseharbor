package machinereadmodels

import (
	"sort"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// ReleaseViewContractVersion versions this read model's shape independently of
// the machine transport contract.
const ReleaseViewContractVersion = "v1"

// UnclearValue is the explicit, readable cell state for a reference dimension
// that was never recorded. A missing version is never shown as an empty cell:
// emptiness reads as "nothing", while UNCLEAR says the source of truth does not
// answer this dimension.
const UnclearValue = "UNCLEAR (no version recorded in this dimension)"

// Reference kind labels. They mirror the three independent dimensions that the
// accepted stage-version writer records in deployment.DeploymentRecord.References.
const (
	referenceIntended = deployment.ReferenceIntended
	referenceApplied  = deployment.ReferenceApplied
	referenceObserved = deployment.ReferenceObserved
)

// ViewStages is the fixed-wanted, fixed-applied, fixed-observed triplet. The
// struct type itself is the separation contract: a change that mixes columns
// has to move values between differently named fields, which the
// wanted/applied/observed assertion matrix rejects.
type ViewStages struct {
	Wanted   *ViewStageCell `json:"wanted,omitempty"`
	Applied  *ViewStageCell `json:"applied,omitempty"`
	Observed *ViewStageCell `json:"observed,omitempty"`
}

// ViewStageCell is exactly one reference dimension rendered as one column.
// Kind is the dimension key it was read from, so no cell can silently report a
// different dimension than it consumed.
type ViewStageCell struct {
	Kind        string    `json:"kind"`
	Reference   string    `json:"reference"`
	ObservedAt  time.Time `json:"observed_at,omitempty"`
	Stale       bool      `json:"stale,omitempty"`
	MissingData bool      `json:"missing_data,omitempty"`
}

// ViewSourceState reports whether the deployment source of truth is readable.
type ViewSourceState struct {
	Available bool   `json:"available"`
	Detail    string `json:"detail,omitempty"`
}

// ViewIdentity is the secret-safe deployment identity a view belongs to.
type ViewIdentity struct {
	Target      string `json:"target"`
	Application string `json:"application"`
	Environment string `json:"environment"`
	SourceKind  string `json:"source_kind,omitempty"`
}

// ComponentVersion is the per-service image repository/tag, observed digest and
// observation freshness for one component of a release.
type ComponentVersion struct {
	Service        string    `json:"service"`
	Image          string    `json:"image,omitempty"`
	Repository     string    `json:"repository,omitempty"`
	Tag            string    `json:"tag,omitempty"`
	Digest         string    `json:"digest,omitempty"`
	Identity       string    `json:"image_id,omitempty"`
	Annotation     string    `json:"version_annotation,omitempty"`
	AnnotationNote string    `json:"annotation_note,omitempty"`
	SourceRevision string    `json:"source_revision,omitempty"`
	Title          string    `json:"title,omitempty"`
	ObservedAt     time.Time `json:"observed_at,omitempty"`
	Stale          bool      `json:"stale,omitempty"`
	Observation    string    `json:"observation"`
}

// VersionView is the readable release/component version view for one
// deployment: the three reference dimensions kept strictly separate, plus the
// per-component image identities and their observation freshness.
type VersionView struct {
	ContractVersion string             `json:"contract_version"`
	Identity        ViewIdentity       `json:"identity"`
	Source          ViewSourceState    `json:"source"`
	Stages          ViewStages         `json:"stages"`
	Components      []ComponentVersion `json:"components,omitempty"`
	RuntimeProvider string             `json:"runtime_provider,omitempty"`
	State           string             `json:"state,omitempty"`
	Ready           bool               `json:"ready,omitempty"`
	VerifiedAt      time.Time          `json:"verified_at,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
}

// ReleaseViewFromRecord derives the version view from one actual deployment
// record. It is read-only: it never writes state and never queries registries.
// The three reference dimensions map one-to-one into the wanted, applied and
// observed cells; a dimension that was not recorded becomes an explicit
// unclear cell instead of a blank.
func ReleaseViewFromRecord(record deployment.DeploymentRecord) VersionView {
	view := VersionView{
		ContractVersion: ReleaseViewContractVersion,
		Identity: ViewIdentity{
			Target:      record.Identity.Target,
			Application: record.Identity.Application,
			Environment: record.Identity.Environment,
			SourceKind:  record.Source.Kind,
		},
		Source: ViewSourceState{Available: deployment.SourceAvailable(record.Source)},
	}
	if !view.Source.Available {
		view.Source.Detail = "deployment source is not readable; shown state is recorded state only"
	}
	view.Stages = ViewStages{
		Wanted:   stageCell(record.References, referenceIntended),
		Applied:  stageCell(record.References, referenceApplied),
		Observed: stageCell(record.References, referenceObserved),
	}
	view.RuntimeProvider = record.Applied.RuntimeProvider
	view.State = record.Observed.State
	view.Ready = record.Observed.Ready
	view.VerifiedAt = record.Observed.VerifiedAt
	// Staleness is the accepted fail-closed comparison of observed against
	// last-applied; no observation counts as stale, never as up to date.
	view.Stages.Observed.Stale = record.Stale() != nil
	return view
}

// UnclearSummary is the compact cell rendering for an unrecorded dimension in
// tabular views.
const UnclearSummary = "UNCLEAR"

// ApplicationVersionsResult is the aggregate read model of separated version
// views across recorded deployments.
type ApplicationVersionsResult struct {
	ContractVersion string        `json:"contract_version"`
	Releases        []VersionView `json:"releases"`
	Warnings        []string      `json:"warnings,omitempty"`
}

// ReleaseViewsFromRecords derives the version view for every record of one
// listing, ordered deterministically by target, application and environment.
func ReleaseViewsFromRecords(records []deployment.DeploymentRecord) []VersionView {
	views := make([]VersionView, 0, len(records))
	for _, record := range records {
		views = append(views, ReleaseViewFromRecord(record))
	}
	sort.SliceStable(views, func(i, j int) bool {
		a, b := views[i].Identity, views[j].Identity
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		if a.Application != b.Application {
			return a.Application < b.Application
		}
		return a.Environment < b.Environment
	})
	return views
}

func stageCell(references map[string]*deployment.ReferenceDimension, kind string) *ViewStageCell {
	cell := &ViewStageCell{Kind: kind}
	dimension, ok := references[kind]
	if !ok || dimension == nil {
		cell.Reference = UnclearValue
		cell.MissingData = true
		return cell
	}
	reference := strings.TrimSpace(dimension.Release)
	if reference == "" {
		cell.Reference = UnclearValue
		cell.MissingData = true
		return cell
	}
	cell.Reference = reference
	cell.ObservedAt = dimension.At
	return cell
}

// SplitImageReference separates an image reference into repository, tag and
// digest without inventing any of them: a digest-only or tag-only reference
// leaves the other empty so the view can mark it unclear rather than guess.
func SplitImageReference(reference string) (repository, tag, digest string) {
	trimmed := strings.TrimSpace(reference)
	rest := trimmed
	if at := strings.IndexByte(rest, '@'); at >= 0 {
		digest = rest[at+1:]
		rest = rest[:at]
	}
	if colon := strings.LastIndexByte(rest, ':'); colon > strings.LastIndexByte(rest, '/') {
		repository = rest[:colon]
		tag = rest[colon+1:]
		return
	}
	repository = rest
	return
}
