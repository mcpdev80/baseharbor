package main

import (
	"context"
)

type ownedDestroyResource struct {
	Target  string `json:"target"`
	Project string `json:"project"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
}

type destroyReport struct {
	Destroyed                         bool                   `json:"destroyed"`
	Preview                           bool                   `json:"preview"`
	All                               bool                   `json:"all"`
	ApplicationSourceAndDataPreserved bool                   `json:"application_source_and_data_preserved"`
	Resources                         []ownedDestroyResource `json:"resources"`
	Preserved                         []fullDestroyResult    `json:"preserved"`
	Results                           []fullDestroyResult    `json:"results"`
}

type destroyReportContextKey struct{}

func withDestroyReport(ctx context.Context, all bool) (context.Context, *destroyReport) {
	report := &destroyReport{Preview: true, All: all, ApplicationSourceAndDataPreserved: true, Resources: []ownedDestroyResource{}, Preserved: []fullDestroyResult{}, Results: []fullDestroyResult{}}
	return context.WithValue(ctx, destroyReportContextKey{}, report), report
}

func recordDestroyPlan(ctx context.Context, plans []targetDestroyInventory, preserved []fullDestroyResult) {
	report, _ := ctx.Value(destroyReportContextKey{}).(*destroyReport)
	if report == nil {
		return
	}
	for _, plan := range plans {
		for _, project := range plan.projectNames() {
			for _, resource := range plan.Projects[project] {
				report.Resources = append(report.Resources, ownedDestroyResource{Target: plan.Target.Name, Project: project, Kind: resource.Kind, Name: resource.Name})
			}
		}
	}
	report.Preserved = append(report.Preserved, preserved...)
}

func recordDestroyResults(ctx context.Context, results []fullDestroyResult, complete bool) {
	report, _ := ctx.Value(destroyReportContextKey{}).(*destroyReport)
	if report == nil {
		return
	}
	report.Results = append(report.Results, results...)
	report.Preview = false
	report.Destroyed = complete
}
