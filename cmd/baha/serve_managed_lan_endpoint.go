package main

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

const managedCoreLANEndpointFile = "managed-serving/lan-endpoint.json"

type ownedCoreLANEndpoint struct {
	Version string `json:"version"`
	Target  string `json:"target"`
	URL     string `json:"url"`
}

// discoverOwnedCoreLANAddress chooses the actual outbound LAN interface when
// available. Dial does not send a packet; it only asks the kernel to choose a
// route. There is never a fallback to a wildcard/loopback address.
func discoverOwnedCoreLANAddress() (string, error) {
	if conn, err := net.Dial("udp4", "192.0.2.1:443"); err == nil {
		ip := conn.LocalAddr().(*net.UDPAddr).IP
		_ = conn.Close()
		if usableOwnedCoreIP(ip) {
			return ip.String(), nil
		}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	matches := map[string]bool{}
	for _, nic := range interfaces {
		if nic.Flags&net.FlagUp == 0 || nic.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := nic.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if usableOwnedCoreIP(ip) {
				matches[ip.String()] = true
			}
		}
	}
	if len(matches) == 1 {
		for address := range matches {
			return address, nil
		}
	}
	return "", fmt.Errorf("no unambiguous routed private IPv4 address for managed Core (candidates: %d); set BASEHARBOR_API_LISTEN_ADDR explicitly", len(matches))
}

func usableOwnedCoreIP(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}
	return ip[0] == 10 ||
		ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 ||
		ip[0] == 192 && ip[1] == 168
}

func ownedCoreLANEndpointPath(target string) (string, error) {
	root, err := deployment.TargetStateRoot(target)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, managedCoreLANEndpointFile), nil
}

func saveOwnedCoreLANEndpoint(target, listenAddr, advertisedHost, certificatePath string) error {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" || !usableNodeHost(host) || !usableNodeHost(advertisedHost) ||
		net.ParseIP(host) == nil || net.ParseIP(host).IsUnspecified() ||
		net.ParseIP(host).IsLoopback() || net.ParseIP(host).IsLinkLocalUnicast() ||
		net.ParseIP(host).IsMulticast() {
		return errors.New("owned Core LAN endpoint requires a concrete listener and advertised host")
	}
	certPEM, err := os.ReadFile(certificatePath)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("managed Core serving certificate is invalid")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil || leaf.VerifyHostname(advertisedHost) != nil {
		return errors.New("managed Core serving certificate does not cover advertised LAN address")
	}
	path, err := ownedCoreLANEndpointPath(target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload, err := json.Marshal(ownedCoreLANEndpoint{
		Version: "baseharbor.core-lan-endpoint/v1",
		Target:  target,
		URL:     "https://" + net.JoinHostPort(advertisedHost, port),
	})
	if err != nil {
		return err
	}
	return replaceManagedCoreTLSFile(path, append(payload, '\n'), 0o600)
}

func loadOwnedCoreLANEndpoint(target string) (string, error) {
	path, err := ownedCoreLANEndpointPath(target)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var endpoint ownedCoreLANEndpoint
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&endpoint); err != nil {
		return "", err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return "", errors.New("managed Core endpoint file contains trailing data")
	}
	if endpoint.Version != "baseharbor.core-lan-endpoint/v1" || endpoint.Target != target ||
		validateCoreURL(endpoint.URL) != nil {
		return "", errors.New("managed Core LAN endpoint belongs to another target or contract")
	}
	hostport := strings.TrimPrefix(endpoint.URL, "https://")
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || host == "" || port == "" || !usableNodeHost(host) {
		return "", errors.New("stored Core LAN endpoint is not a concrete HTTPS origin")
	}
	numericPort, err := strconv.Atoi(port)
	if err != nil || numericPort < 1 || numericPort > 65535 {
		return "", errors.New("stored Core LAN endpoint is not a concrete HTTPS origin")
	}
	return endpoint.URL, nil
}
