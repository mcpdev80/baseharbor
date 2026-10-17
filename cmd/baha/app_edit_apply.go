package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

type appEditDesired struct {
	Source        []byte
	Manifest      application.Manifest
	Target        string
	ApplicationID string
	Requirements  []capability.Requirement
	Preferences   []application.ProviderPreference
}

type appEditApplyResult struct {
	Changed      bool
	Source       []byte
	Plan         application.ProviderImpactPlan
	Result       application.ProviderChangeResult
	Presentation string
}

func appEditParseSource(loaded appEditLoaded) (application.Manifest, error) {
	manifest, err := application.ParseYAML(string(loaded.Source))
	if err != nil {
		manifest, err = application.ParseCanonicalYAML(string(loaded.Source))
		if err != nil {
			return application.Manifest{}, err
		}
	}
	manifest.ApplicationID = loaded.ApplicationID
	return manifest, nil
}

// appEditDesiredFromDecision changes only wizard-owned portable requirements.
// Other manifest intent is copied from the hydrated source. Rich declarations
// (HTTP, metrics, logs, OTLP) may be removed but are never invented from a bare
// checkbox because their source fields are not represented by the compact UI.
// appEditProjectRequirements projects the wizard-owned portable requirements
// onto a fresh copy of the authoritative manifest. Other manifest intent is
// carried unchanged. Rich declarations (HTTP, metrics, logs, OTLP) may be
// retained but are never invented from a bare checkbox because their source
// fields are not represented by the compact UI.
func appEditProjectRequirements(original application.Manifest, requirements []capability.Requirement) (application.Manifest, error) {
	manifest := original
	manifest.Services.SQL, manifest.Services.Cache, manifest.Services.KeyValue = false, false, false
	manifest.Services.DocumentDatabase = false
	manifest.Services.MessagingQueue, manifest.Services.MessagingPubSub, manifest.Services.MessagingStream = false, false, false
	manifest.Services.ObjectStorage, manifest.Services.Secrets, manifest.Services.Identity = false, false, false
	manifest.Services.SQLInstances = nil
	manifest.Services.CacheInstances = nil
	manifest.Services.KeyValueInstances = nil
	manifest.Services.DocumentDatabaseInstances = nil
	manifest.Services.MessagingQueueInstances = nil
	manifest.Services.MessagingPubSubInstances = nil
	manifest.Services.MessagingStreamInstances = nil
	manifest.Services.ObjectStorageBuckets = nil
	manifest.Exposures = nil
	manifest.Metrics.Sources = nil
	manifest.Logs.Collect = nil
	manifest.Telemetry.OTLP = nil

	seen := map[capability.Requirement]bool{}
	for _, requirement := range requirements {
		if requirement.Kind == "" || requirement.Name == "" || seen[requirement] {
			return application.Manifest{}, fmt.Errorf("invalid or duplicate requirement %s/%s", requirement.Kind, requirement.Name)
		}
		seen[requirement] = true
		switch requirement.Kind {
		case capability.SQL:
			manifest = application.WithSQLInstances(manifest, requirement.Name)
		case capability.KeyValue:
			manifest = application.WithCacheInstances(manifest, requirement.Name)
		case capability.DurableKeyValue:
			manifest = application.WithKeyValueInstances(manifest, requirement.Name)
		case capability.DocumentDatabase:
			manifest = application.WithDocumentDatabaseInstances(manifest, requirement.Name)
		case capability.MessagingQueue:
			manifest = application.WithMessagingQueueInstances(manifest, requirement.Name)
		case capability.MessagingPubSub:
			manifest = application.WithMessagingPubSubInstances(manifest, requirement.Name)
		case capability.MessagingStream:
			manifest = application.WithMessagingStreamInstances(manifest, requirement.Name)
		case capability.ObjectStorageS3:
			manifest = application.WithObjectStorageBuckets(manifest, requirement.Name)
		case capability.Secrets:
			manifest.Services.Secrets = true
		case capability.Identity:
			manifest.Services.Identity = true
		case capability.ExposureHTTP:
			item, ok := appEditExposure(original, requirement.Name)
			if !ok {
				return application.Manifest{}, fmt.Errorf("HTTP exposure %q cannot be invented by app edit", requirement.Name)
			}
			manifest.Exposures = append(manifest.Exposures, item)
		case capability.Metrics:
			item, ok := appEditMetric(original, requirement.Name)
			if !ok {
				return application.Manifest{}, fmt.Errorf("metrics source %q cannot be invented by app edit", requirement.Name)
			}
			manifest.Metrics.Sources = append(manifest.Metrics.Sources, item)
		case capability.Logs:
			if !containsString(original.Logs.Collect, requirement.Name) {
				return application.Manifest{}, fmt.Errorf("log source %q cannot be invented by app edit", requirement.Name)
			}
			manifest.Logs.Collect = append(manifest.Logs.Collect, requirement.Name)
		case capability.TelemetryOTLP:
			if original.Telemetry.OTLP == nil {
				return application.Manifest{}, errors.New("OTLP details cannot be invented by app edit")
			}
			copyOTLP := *original.Telemetry.OTLP
			copyOTLP.Signals = append([]string(nil), original.Telemetry.OTLP.Signals...)
			manifest.Telemetry.OTLP = &copyOTLP
		case capability.Traces:
			return application.Manifest{}, errors.New("standalone traces selection is not representable by the current manifest contract")
		default:
			return application.Manifest{}, fmt.Errorf("unsupported edit requirement %s/%s", requirement.Kind, requirement.Name)
		}
	}

	// Existing detailed intent remains attached whenever its corresponding
	// compact need stays selected.
	if manifest.Services.Identity {
		manifest.Identity = original.Identity
	}
	if manifest.Services.Secrets {
		manifest.Secrets = original.Secrets
	} else if len(original.Secrets.Required) > 0 || len(original.Secrets.Optional) > 0 {
		return application.Manifest{}, errors.New("managed secrets cannot be deselected while authored secret requirements remain")
	}
	return manifest, nil
}

func appEditDesiredFromDecision(loaded appEditLoaded, decision appWizardDecision) (appEditDesired, error) {
	if decision.Target == "" || decision.Target != loaded.Target {
		return appEditDesired{}, errors.New("confirmed edit decision has the wrong Target")
	}
	manifest, err := appEditParseSource(loaded)
	if err != nil {
		return appEditDesired{}, err
	}
	manifest, err = appEditProjectRequirements(manifest, decision.Requirements)
	if err != nil {
		return appEditDesired{}, err
	}

	preferences := make([]application.ProviderPreference, 0, len(decision.Preferences))
	seen := map[capability.Requirement]bool{}
	for _, requirement := range decision.Requirements {
		seen[requirement] = true
	}
	for _, preference := range decision.Preferences {
		if preference.Target != decision.Target || !seen[preference.Requirement] {
			return appEditDesired{}, errors.New("provider preference is not bound to the confirmed Target and requirement")
		}
		preferences = append(preferences, preference.ProviderPreference)
	}
	if err := manifest.Validate(); err != nil {
		return appEditDesired{}, err
	}
	return appEditDesired{
		Source:        bytes.Clone(loaded.Source),
		Manifest:      manifest,
		Target:        decision.Target,
		ApplicationID: loaded.ApplicationID,
		Requirements:  append([]capability.Requirement(nil), decision.Requirements...),
		Preferences:   preferences,
	}, nil
}

func appEditExposure(manifest application.Manifest, name string) (application.HTTPExposureRequirement, bool) {
	for _, item := range manifest.Exposures {
		if item.Name == name {
			return item, true
		}
	}
	return application.HTTPExposureRequirement{}, false
}

func appEditMetric(manifest application.Manifest, name string) (application.MetricsSourceRequirement, bool) {
	for _, item := range manifest.Metrics.Sources {
		if item.Name == name {
			return item, true
		}
	}
	return application.MetricsSourceRequirement{}, false
}

func appEditEquivalent(loaded appEditLoaded, desired appEditDesired) bool {
	if loaded.Target != desired.Target || loaded.ApplicationID != desired.ApplicationID {
		return false
	}
	return reflect.DeepEqual(appEditSortedRequirements(loaded.Capabilities), appEditSortedRequirements(desired.Requirements)) &&
		reflect.DeepEqual(appEditSortedPreferences(loaded.Preferences), appEditSortedPreferences(desired.Preferences))
}

func appEditSortedRequirements(in []capability.Requirement) []capability.Requirement {
	out := append([]capability.Requirement(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func appEditSortedPreferences(in []application.ProviderPreference) []application.ProviderPreference {
	out := append([]application.ProviderPreference(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requirement.Kind == out[j].Requirement.Kind {
			if out[i].Requirement.Name == out[j].Requirement.Name {
				return out[i].InstanceID < out[j].InstanceID
			}
			return out[i].Requirement.Name < out[j].Requirement.Name
		}
		return out[i].Requirement.Kind < out[j].Requirement.Kind
	})
	return out
}

// appEditApplyConfirmed is the only Edit path that can reach mutation. The
// accepted machine adapter re-reads, replans, re-authorizes and validates the
// exact consent immediately before Execute. Stale plans and foreign ownership
// therefore fail before source/runtime mutation.
func appEditApplyConfirmed(ctx context.Context, controller appWizardController, loaded appEditLoaded, consent application.ProviderChangeConsent, authorities appEditAuthorities) (appEditApplyResult, error) {
	decision, confirmed := controller.Decision()
	if !confirmed {
		return appEditApplyResult{Source: appEditLoadSource(loaded)}, errAppWizardAbort
	}
	if authorities.Read == nil || authorities.Choices == nil || authorities.Plan == nil || authorities.Authorize == nil || authorities.Execute == nil {
		return appEditApplyResult{Source: appEditLoadSource(loaded)}, errors.New("app edit mutation authorities are incomplete")
	}
	desired, err := appEditDesiredFromDecision(loaded, decision)
	if err != nil {
		return appEditApplyResult{Source: appEditLoadSource(loaded)}, err
	}
	if appEditEquivalent(loaded, desired) {
		return appEditApplyResult{Source: appEditLoadSource(loaded), Changed: false, Presentation: "No application changes requested.\n"}, nil
	}
	fresh, err := authorities.Read(ctx, decision.Target)
	if err != nil {
		return appEditApplyResult{Source: appEditLoadSource(loaded)}, err
	}
	request := appEditMachineRequest(decision, fresh.SnapshotRevision)
	proposal, err := appMachinePropose(ctx, request, authorities.Read, authorities.Choices, authorities.Plan)
	if err != nil {
		return appEditApplyResult{Source: appEditLoadSource(loaded)}, err
	}
	machineApply := func(applyCtx context.Context, plan application.ProviderImpactPlan, approved application.ProviderChangeConsent) ([]application.ProviderExecutionOutcome, error) {
		return authorities.Execute(applyCtx, desired, plan, approved)
	}
	result, changeErr := appMachineChangeDesired(ctx, appMachineDesiredChangeRequest{
		Desired: request, Proposal: proposal, Consent: consent,
	}, authorities.Read, authorities.Choices, authorities.Plan, authorities.Authorize, machineApply)
	presentation, presentErr := appChangeResultPresenter(proposal.Plan, result)
	if presentErr != nil {
		if changeErr != nil {
			return appEditApplyResult{Source: appEditLoadSource(loaded), Changed: true, Plan: proposal.Plan, Result: result}, errors.Join(changeErr, presentErr)
		}
		return appEditApplyResult{Source: appEditLoadSource(loaded), Changed: true, Plan: proposal.Plan, Result: result}, presentErr
	}
	output := appEditApplyResult{
		Source:       appEditLoadSource(loaded),
		Changed:      true,
		Plan:         proposal.Plan,
		Result:       result,
		Presentation: presentation,
	}
	return output, changeErr
}

// runAppEditCommand consumes an already decided accepted controller. Central
// command registration is deliberately owned by core-921. Consequence consent
// is collected with the existing authority; No/Cancel/EOF never reach Execute.
func runAppEditCommand(ctx context.Context, session appEditSession, in io.Reader, out io.Writer, authorities appEditAuthorities) (appEditApplyResult, error) {
	decision, confirmed := session.Controller.Decision()
	if !confirmed {
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, errAppWizardAbort
	}
	desired, err := appEditDesiredFromDecision(session.Loaded, decision)
	if err != nil {
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, err
	}
	if appEditEquivalent(session.Loaded, desired) {
		return appEditApplyConfirmed(ctx, session.Controller, session.Loaded, application.ProviderChangeConsent{}, authorities)
	}
	if session.Controller.impact == nil {
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, errors.New("confirmed edit has no current provider impact preview")
	}
	if out == nil {
		out = io.Discard
	}
	if in == nil {
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, errAppWizardAbort
	}
	consent, err := readProviderImpactAcknowledgements(bufio.NewReader(in), out, *session.Controller.impact)
	if err != nil {
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, err
	}
	if err := application.ValidateProviderChangeConsent(*session.Controller.impact, consent); err != nil {
		return appEditApplyResult{Source: appEditLoadSource(session.Loaded)}, classifyProviderChangeConsent(err)
	}
	return appEditApplyConfirmed(ctx, session.Controller, session.Loaded, consent, authorities)
}
