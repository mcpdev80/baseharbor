package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestBuildWorkloadServiceStatuses(t *testing.T) {
	statuses := buildWorkloadServiceStatuses(
		[]string{"worker", "edge", "api", "web"},
		[]bhruntime.ServiceState{
			{Service: "api", State: "running", Health: "healthy"},
			{Service: "edge", State: "running"},
			{Service: "web", State: "running", Health: "starting"},
		},
	)
	if len(statuses) != 4 {
		t.Fatalf("got %d statuses, want 4", len(statuses))
	}
	if statuses[0].Service != "api" || !statuses[0].Ready || statuses[0].Health != "healthy" {
		t.Fatalf("unexpected api status: %#v", statuses[0])
	}
	if statuses[1].Service != "edge" || !statuses[1].Ready {
		t.Fatalf("unexpected edge status: %#v", statuses[1])
	}
	if statuses[2].Service != "web" || statuses[2].Ready || statuses[2].Health != "starting" {
		t.Fatalf("unexpected web status: %#v", statuses[2])
	}
	if statuses[3].Service != "worker" || statuses[3].Ready || statuses[3].State != "not running" {
		t.Fatalf("unexpected worker status: %#v", statuses[3])
	}
}

func TestRepositoryWorkloadStatusReadyIncludesExposure(t *testing.T) {
	services := []workloadServiceStatus{
		{Service: "api", State: "running", Ready: true},
		{Service: "web", State: "running", Ready: true},
	}
	exposures := []workloadExposureStatus{{Service: "web", Scheme: "https", Host: "127.0.0.1", Port: 443, Ready: true, Detail: "HTTP 200"}}
	status := repositoryWorkloadStatus{Found: true, Services: attachWorkloadExposures(services, exposures), Exposures: exposures}
	if !status.Ready() || status.ReadyCount() != 2 || status.ExposureReadyCount() != 1 {
		t.Fatalf("expected ready status: %#v", status)
	}

	failedExposures := []workloadExposureStatus{{Service: "web", Scheme: "https", Host: "127.0.0.1", Port: 443, Ready: false, Detail: "unreachable"}}
	status = repositoryWorkloadStatus{
		Found: true,
		Services: attachWorkloadExposures([]workloadServiceStatus{
			{Service: "api", State: "running", Ready: true},
			{Service: "web", State: "running", Ready: true},
		}, failedExposures),
		Exposures: failedExposures,
	}
	if status.Ready() || status.ReadyCount() != 1 || status.ExposureReadyCount() != 0 {
		t.Fatalf("expected exposure failure: %#v", status)
	}
	if err := workloadExposureReadinessError(status.Exposures); err == nil || !strings.Contains(err.Error(), "exposure readiness failed") {
		t.Fatalf("expected classified exposure error, got %v", err)
	}
	if got := formatWorkloadServiceStatus(status.Services[1]); !strings.Contains(got, "exposure=https://127.0.0.1:443 unreachable") {
		t.Fatalf("expected exposure detail in service status, got %q", got)
	}
}

func TestWorkloadExposureScheme(t *testing.T) {
	for _, test := range []struct {
		target, published int
		scheme            string
		ok                bool
	}{
		{443, 443, "https", true},
		{443, 18443, "https", true},
		{8080, 18080, "http", true},
		{3000, 3000, "http", true},
		{5432, 15432, "", false},
	} {
		scheme, ok := workloadExposureScheme(test.target, test.published)
		if scheme != test.scheme || ok != test.ok {
			t.Fatalf("workloadExposureScheme(%d,%d) = %q,%v; want %q,%v", test.target, test.published, scheme, ok, test.scheme, test.ok)
		}
	}
}

func TestProbeHTTPExposureAcceptsHTTPRedirectAndSelfSignedTLS(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.invalid/", http.StatusPermanentRedirect)
	}))
	defer httpServer.Close()
	host, port := testServerHostPort(t, httpServer.URL)
	ready, detail := probeHTTPExposure(context.Background(), "http", host, port)
	if !ready || detail != "HTTP 308" {
		t.Fatalf("HTTP redirect readiness = %v %q", ready, detail)
	}

	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer tlsServer.Close()
	host, port = testServerHostPort(t, tlsServer.URL)
	ready, detail = probeHTTPExposure(context.Background(), "https", host, port)
	if !ready || detail != "HTTP 200" {
		t.Fatalf("TLS readiness = %v %q", ready, detail)
	}
}

func TestProbeHTTPExposureRejectsServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	host, port := testServerHostPort(t, server.URL)
	ready, detail := probeHTTPExposure(context.Background(), "http", host, port)
	if ready || detail != "HTTP 503" {
		t.Fatalf("server failure readiness = %v %q", ready, detail)
	}
}

func testServerHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func TestFormatWorkloadServiceStatus(t *testing.T) {
	if got := formatWorkloadServiceStatus(workloadServiceStatus{State: "running", Health: "healthy"}); got != "running health=healthy" {
		t.Fatalf("got %q", got)
	}
	if got := formatWorkloadServiceStatus(workloadServiceStatus{State: "not running"}); got != "not running" {
		t.Fatalf("got %q", got)
	}
}
