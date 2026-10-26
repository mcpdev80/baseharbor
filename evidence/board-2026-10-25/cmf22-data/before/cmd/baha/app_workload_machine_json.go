package main

import "encoding/json"

// MarshalJSON keeps legacy runtime metadata internal and publishes stable,
// version-neutral snake_case nested data through app.show/CLI/MCP.
func (s repositoryWorkloadStatus) MarshalJSON() ([]byte, error) {
	type workloadFiles struct {
		RepositoryRoot string   `json:"repository_root"`
		Compose        string   `json:"compose"`
		Override       string   `json:"override"`
		Services       []string `json:"services"`
		Project        string   `json:"project"`
		Partial        bool     `json:"partial"`
	}
	type payload struct {
		Found       bool                     `json:"found"`
		Workload    workloadFiles            `json:"workload"`
		Services    []workloadServiceStatus  `json:"services"`
		Exposures   []workloadExposureStatus `json:"exposures"`
		BuildDrift  []string                 `json:"build_drift"`
		ConfigDrift []string                 `json:"config_drift"`
	}
	files := s.Workload
	if files.Services == nil {
		files.Services = []string{}
	}
	services := s.Services
	if services == nil {
		services = []workloadServiceStatus{}
	}
	exposures := s.Exposures
	if exposures == nil {
		exposures = []workloadExposureStatus{}
	}
	buildDrift := s.BuildDrift
	if buildDrift == nil {
		buildDrift = []string{}
	}
	configDrift := s.ConfigDrift
	if configDrift == nil {
		configDrift = []string{}
	}
	return json.Marshal(payload{
		Found:    s.Found,
		Workload: workloadFiles{RepositoryRoot: files.RepositoryRoot, Compose: files.Compose, Override: files.Override, Services: files.Services, Project: files.Project, Partial: files.Partial},
		Services: services, Exposures: exposures, BuildDrift: buildDrift, ConfigDrift: configDrift,
	})
}
