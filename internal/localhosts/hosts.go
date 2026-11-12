// Package localhosts owns explicit, target-scoped workstation hosts blocks.
package localhosts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
)

type Change struct {
	ID       string `json:"id"`
	Previous string `json:"previous"`
	Next     string `json:"next"`
}

var identifier = regexp.MustCompile(`^[a-f0-9]{32}$`)
var hostname = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

func ID(workstation, target string) string {
	sum := sha256.Sum256([]byte(workstation + "\x00" + target))
	return hex.EncodeToString(sum[:16])
}

func Block(id, address string, names []string) (string, error) {
	if !identifier.MatchString(id) {
		return "", errors.New("invalid hosts ownership ID")
	}
	ip := net.ParseIP(address)
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return "", errors.New("hosts entries require a concrete IP address")
	}
	unique := map[string]bool{}
	for _, name := range names {
		if len(name) > 253 || !hostname.MatchString(name) || net.ParseIP(name) != nil || name == "localhost" || strings.HasSuffix(name, ".localhost") {
			return "", fmt.Errorf("unsupported local hosts name %q", name)
		}
		for _, label := range strings.Split(name, ".") {
			if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return "", errors.New("invalid hosts label")
			}
		}
		unique[name] = true
	}
	sorted := make([]string, 0, len(unique))
	for name := range unique {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	if len(sorted) == 0 {
		return "", nil
	}
	body := "# BEGIN BaseHarbor " + id + "\n"
	for _, name := range sorted {
		body += ip.String() + "\t" + name + "\n"
	}
	return body + "# END BaseHarbor " + id + "\n", nil
}

// Rewrite preserves every byte outside the exact previously owned block.
func Rewrite(current string, change Change) (string, error) {
	if !identifier.MatchString(change.ID) {
		return "", errors.New("invalid hosts ownership ID")
	}
	if err := validateBlock(change.ID, change.Previous); err != nil {
		return "", err
	}
	if err := validateBlock(change.ID, change.Next); err != nil {
		return "", err
	}
	begin := "# BEGIN BaseHarbor " + change.ID
	end := "# END BaseHarbor " + change.ID
	if change.Previous != "" {
		if strings.Count(current, begin) != 1 || strings.Count(current, end) != 1 || strings.Count(current, change.Previous) != 1 {
			return "", errors.New("PRESERVED: owned hosts block was removed or modified; inspect /etc/hosts")
		}
	} else if strings.Contains(current, begin) || strings.Contains(current, end) {
		return "", errors.New("PRESERVED: hosts ownership is not recorded")
	}
	outside := strings.Replace(current, change.Previous, "", 1)
	// Refuse conflicting names, including blocks belonging to another Target.
	for _, line := range strings.Split(change.Next, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.HasPrefix(line, "#") {
			continue
		}
		for _, existing := range strings.Split(outside, "\n") {
			columns := strings.Fields(strings.SplitN(existing, "#", 2)[0])
			if len(columns) < 2 {
				continue
			}
			for _, name := range columns[1:] {
				if strings.EqualFold(name, fields[1]) && !net.ParseIP(columns[0]).Equal(net.ParseIP(fields[0])) {
					return "", fmt.Errorf("PRESERVED: hosts name %s already exists outside this Target", name)
				}
			}
		}
	}
	if change.Previous != "" {
		return strings.Replace(current, change.Previous, change.Next, 1), nil
	}
	if change.Next == "" {
		return current, nil
	}
	if current != "" && !strings.HasSuffix(current, "\n") {
		return "", errors.New("hosts file has no final newline; preserve and correct it before retrying")
	}
	return current + change.Next, nil
}

func validateBlock(id, block string) error {
	if block == "" {
		return nil
	}
	lines := strings.Split(block, "\n")
	if len(lines) < 4 || lines[0] != "# BEGIN BaseHarbor "+id || lines[len(lines)-2] != "# END BaseHarbor "+id || lines[len(lines)-1] != "" {
		return errors.New("invalid hosts block")
	}
	var names []string
	address := ""
	for _, line := range lines[1 : len(lines)-2] {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return errors.New("invalid hosts entry")
		}
		if address != "" && address != fields[0] {
			return errors.New("mixed hosts addresses")
		}
		address = fields[0]
		names = append(names, fields[1])
	}
	normalized, err := Block(id, address, names)
	if err != nil {
		return err
	}
	if normalized != block {
		return errors.New("noncanonical hosts block")
	}
	return nil
}
