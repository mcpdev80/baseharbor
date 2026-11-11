package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func remoteCoreIPAccessAddress(target deployment.ResolvedTarget) (string, error) {
	path := strings.TrimSpace(os.Getenv("BASEHARBOR_CONNECTION_PROFILE"))
	if path == "" {
		root, err := deployment.DataRoot()
		if err != nil {
			return "", err
		}
		path = filepath.Join(root, "remote-connections", target.Name, "profile.json")
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("open remote Core profile for development gateway IP access: %w", err)
	}
	defer file.Close()
	var profile remoteConnectionProfile
	decoder := json.NewDecoder(io.LimitReader(file, 128*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return "", fmt.Errorf("decode remote Core profile for development gateway IP access: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", errors.New("remote Core profile contains trailing data")
	}
	if profile.Version != remoteConnectionProfileVersion {
		return "", errors.New("unsupported remote Core profile for development gateway IP access")
	}
	selected := false
	for _, candidate := range profile.Targets {
		if candidate.Name == target.Name && candidate.RuntimeProvider == target.RuntimeProvider {
			selected = true
			break
		}
	}
	if !selected {
		return "", nil
	}
	origin, err := url.Parse(profile.Core.URL)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return "", errors.New("remote Core profile has an invalid HTTPS origin for IP access")
	}
	host := origin.Hostname()
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() {
		return ip.String(), nil
	}
	addresses, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("resolve remote Core host for direct management access: %w", err)
	}
	var selectedIP net.IP
	for _, address := range addresses {
		ipv4 := address.To4()
		if ipv4 == nil || ipv4.IsLoopback() || ipv4.IsUnspecified() || ipv4.IsLinkLocalUnicast() || ipv4.IsMulticast() {
			continue
		}
		if selectedIP != nil && !selectedIP.Equal(ipv4) {
			return "", errors.New("remote Core hostname resolves to multiple IPv4 addresses; configure a single direct IP endpoint")
		}
		selectedIP = ipv4
	}
	if selectedIP == nil {
		return "", nil
	}
	return selectedIP.String(), nil
}
