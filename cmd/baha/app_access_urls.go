package main

import (
	"net"
	"net/url"
	"strconv"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/exposure"
)

// Links use the owning Core's advertised address and the actual exposure ports,
// never a workstation address, wildcard bind, guessed port or internal route.
func applicationAccessURLs(resolved resolvedApplication, ready bool) []string {
	if !ready {
		return nil
	}
	address, err := loadOwnedCoreLANEndpoint(resolved.Target.Name)
	if err != nil {
		return nil
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return nil
	}
	state, _, err := exposure.Load(files)
	if err != nil {
		return nil
	}
	return publicApplicationAccessURLs(address, state)
}

func publicApplicationAccessURLs(address string, state exposure.State) []string {
	core, err := url.Parse(address)
	if err != nil || core.User != nil || !usableOwnedCoreIP(net.ParseIP(core.Hostname())) {
		return nil
	}
	var urls []string
	seen := map[string]bool{}
	for _, route := range state.Routes {
		if route.Visibility != "public" || route.PublishedPort < 1 || route.PublishedPort > 65535 {
			continue
		}
		protocol := route.ProviderProtocol
		if protocol == "" {
			protocol = route.Protocol
		}
		if protocol != "http" && protocol != "https" {
			continue
		}
		endpoint := url.URL{Scheme: protocol, Host: net.JoinHostPort(core.Hostname(), strconv.Itoa(route.PublishedPort))}
		if !seen[endpoint.String()] {
			urls = append(urls, endpoint.String())
			seen[endpoint.String()] = true
		}
	}
	return urls
}
