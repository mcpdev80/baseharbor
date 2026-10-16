package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
)

// appWizardPhase is a read-only navigation state, not a lifecycle stage.
type appWizardPhase string

const (
	appWizardNeeds    appWizardPhase = "needs"
	appWizardProvider appWizardPhase = "providers"
	appWizardPreview  appWizardPhase = "preview"
	appWizardDecided  appWizardPhase = "decided"
	appWizardAborted  appWizardPhase = "aborted"
)

var errAppWizardAbort = errors.New("wizard cancelled without changes")
var errAppWizardNonInteractive = errors.New("interactive wizard requires a TTY and input")

type appWizardEvent string

const (
	appWizardNext   appWizardEvent = "next"
	appWizardBack   appWizardEvent = "back"
	appWizardCancel appWizardEvent = "cancel"
	appWizardEOF    appWizardEvent = "eof"
	appWizardNo     appWizardEvent = "no"
	appWizardYes    appWizardEvent = "yes"
)

type appWizardInput struct {
	Manifest application.Manifest
	Snapshot application.ResolutionSnapshot
	Selection appCapabilitySelectionInput
	// The caller supplies domain-generated, immutable projections. This
	// controller neither computes impact nor authorizes their execution.
	Footprint *GuidedReadContext
	Impact *application.ProviderImpactPlan
	TTY bool
	NoInput bool
}

type appWizardDecision struct {
	Target string
	Requirements []capability.Requirement
	Preferences []appProviderPreference
	// A decision is intent only. No execute callback is exposed here.
}

// appWizardController contains no filesystem, process or domain mutation hook.
type appWizardController struct {
	phase appWizardPhase
	manifest application.Manifest
	snapshot application.ResolutionSnapshot
	selection appCapabilitySelectionModel
	choices []appProviderChoice
	preferences map[string]string
	footprint *GuidedReadContext
	impact *application.ProviderImpactPlan
	decision *appWizardDecision
}

func newAppWizardController(ctx context.Context, input appWizardInput) (appWizardController, error) {
	if err := ctx.Err(); err != nil { return appWizardController{}, err }
	if !input.TTY || input.NoInput { return appWizardController{}, errAppWizardNonInteractive }
	if input.Snapshot.Target == "" { return appWizardController{}, errors.New("wizard Target required") }
	return appWizardController{
		phase: appWizardNeeds,
		manifest: input.Manifest,
		snapshot: input.Snapshot,
		selection: newAppCapabilitySelection(input.Selection),
		preferences: map[string]string{},
		footprint: input.Footprint,
		impact: input.Impact,
	}, nil
}

func appWizardKey(target string, req capability.Requirement) string {
	return fmt.Sprintf("%d:%s|%d:%s|%d:%s",len(target),target,len(req.Kind),req.Kind,len(req.Name),req.Name)
}

func (w appWizardController) copy() appWizardController {
	next := w
	next.selection.selected = make(map[capability.Kind]bool,len(w.selection.selected))
	for k,v := range w.selection.selected { next.selection.selected[k] = v }
	next.selection.detected = make(map[capability.Kind]bool,len(w.selection.detected))
	for k,v := range w.selection.detected { next.selection.detected[k] = v }
	next.selection.current = append([]capability.Requirement(nil),w.selection.current...)
	next.choices = append([]appProviderChoice(nil),w.choices...)
	next.preferences = make(map[string]string,len(w.preferences))
	for k,v := range w.preferences { next.preferences[k] = v }
	if w.decision != nil {
		d := *w.decision
		d.Requirements = append([]capability.Requirement(nil),d.Requirements...)
		d.Preferences = append([]appProviderPreference(nil),d.Preferences...)
		next.decision = &d
	}
	return next
}

func (w appWizardController) abort() appWizardController {
	next := w.copy()
	next.phase = appWizardAborted
	next.decision = nil
	return next
}

// Step is pure navigation. EOF/cancel/context errors are terminal. No
// confirmation or preview event invokes a lifecycle or provider operation.
func (w appWizardController) Step(ctx context.Context, event appWizardEvent) (appWizardController,error) {
	if err := ctx.Err(); err != nil { return w.abort(),err }
	if w.phase == appWizardAborted { return w,errAppWizardAbort }
	if w.phase == appWizardDecided { return w,errors.New("decision already returned") }
	if event == appWizardCancel || event == appWizardEOF { return w.abort(),errAppWizardAbort }
	next := w.copy()
	switch w.phase {
	case appWizardNeeds:
		switch event {
		case appWizardBack,appWizardNo:
			return next,nil
		case appWizardNext:
			choices,err := appProviderChoices(next.manifest,next.snapshot,next.selection)
			if err != nil { return w,err }
			for i := range choices {
				if id,ok := next.preferences[appWizardKey(next.snapshot.Target,choices[i].Requirement)];ok {
					chosen,err := choices[i].Select(id)
					if err != nil { return w,err }
					choices[i] = chosen
				}
			}
			next.choices = choices
			next.phase = appWizardProvider
		}
	case appWizardProvider:
		switch event {
		case appWizardBack,appWizardNo:
			next.phase = appWizardNeeds
		case appWizardNext:
			next.phase = appWizardPreview
		}
	case appWizardPreview:
		switch event {
		case appWizardBack,appWizardNo:
			next.phase = appWizardProvider
		case appWizardYes:
			decision := &appWizardDecision{Target:next.snapshot.Target,
				Requirements:append([]capability.Requirement(nil),next.selection.Requirements()...)}
			for _,choice := range next.choices {
				decision.Preferences = append(decision.Preferences,choice.Preference())
			}
			next.decision = decision
			next.phase = appWizardDecided
		}
	}
	return next,nil
}

// SelectNeed delegates to the accepted needs component; it never confirms a
// lifecycle operation. Return navigation remains separate from selection.
func (w appWizardController) SelectNeed(ctx context.Context,key string) (appWizardController,error) {
	if err := ctx.Err();err != nil { return w.abort(),err }
	if w.phase != appWizardNeeds { return w,errors.New("not on needs screen") }
	next := w.copy()
	var confirmed bool
	next.selection,confirmed = next.selection.Update(key)
	if confirmed {
		// A component's Enter only confirms its own screen; lifecycle
		// advancement is always an explicit controller Step(next).
		next.selection.done = false
	}
	return next,nil
}

func (w appWizardController) SelectProvider(ctx context.Context,requirement capability.Requirement,instanceID string) (appWizardController,error) {
	if err := ctx.Err();err != nil { return w.abort(),err }
	if w.phase != appWizardProvider { return w,errors.New("not on provider screen") }
	next := w.copy()
	for i,choice := range next.choices {
		if choice.Requirement != requirement { continue }
		selected,err := choice.Select(instanceID)
		if err != nil { return w,err }
		next.choices[i] = selected
		key := appWizardKey(next.snapshot.Target,requirement)
		if instanceID == "" { delete(next.preferences,key) } else { next.preferences[key] = instanceID }
		return next,nil
	}
	return w,fmt.Errorf("requirement %s/%s not in current selection",requirement.Kind,requirement.Name)
}

func (w appWizardController) View() string {
	switch w.phase {
	case appWizardNeeds:
		return w.selection.View()
	case appWizardProvider:
		out := ""
		for _,choice := range w.choices { out += choice.View() + "\n" }
		return out
	case appWizardPreview:
		out := "Read-only decision preview (not execution)\n"
		if w.footprint != nil { out += presentAppFootprint(*w.footprint,appFootprintPresenterOptions{}) + "\n" }
		if w.impact != nil { out += appChangePreview(*w.impact) }
		return out
	case appWizardDecided:
		return "Decision returned to caller; execution requires separate authorized lifecycle."
	default:
		return "Wizard cancelled without changes."
	}
}

func (w appWizardController) Decision() (appWizardDecision,bool) {
	if w.phase != appWizardDecided || w.decision == nil { return appWizardDecision{},false }
	d := *w.decision
	d.Requirements = append([]capability.Requirement(nil),d.Requirements...)
	d.Preferences = append([]appProviderPreference(nil),d.Preferences...)
	return d,true
}
