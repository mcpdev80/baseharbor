package main

import "testing"

func TestApplicationComponentsStopped(t *testing.T) {
	tests := []struct {
		name            string
		managed         []string
		workload        []string
		workloadFound   bool
		brokerRunning   bool
		exposureRunning bool
		want            bool
	}{
		{name: "fully stopped managed app", workloadFound: true, want: true},
		{name: "workload only stopped app", workloadFound: true, want: true},
		{name: "managed backend running", managed: []string{"postgres"}, workloadFound: true, want: false},
		{name: "workload running", workload: []string{"web"}, workloadFound: true, want: false},
		{name: "broker running", workloadFound: true, brokerRunning: true, want: false},
		{name: "managed exposure running", workloadFound: true, exposureRunning: true, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := applicationComponentsStopped(test.managed, test.workload, test.workloadFound, test.brokerRunning, test.exposureRunning); got != test.want {
				t.Fatalf("applicationComponentsStopped() = %v, want %v", got, test.want)
			}
		})
	}
}
