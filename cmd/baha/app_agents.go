package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	baseHarborAgentsStart = "<!-- baseharbor:start -->"
	baseHarborAgentsEnd   = "<!-- baseharbor:end -->"
)

func baseHarborAgentsSection() string {
	return baseHarborAgentsStart + "\n" +
		"## BaseHarbor\n\n" +
		"- Use BaseHarbor for infrastructure managed by this project.\n" +
		"- Run `baha app inspect .` and `baha app plan` before infrastructure mutation.\n" +
		"- Keep provider, placement and runtime topology out of portable `baseharbor.yaml`; declare application requirements instead.\n" +
		"- Never place secret or credential values in `baseharbor.yaml`.\n" +
		"- Never bypass BaseHarbor security, ownership or provider-binding boundaries.\n" +
		"- Prefer BaseHarbor structured interfaces for automation instead of parsing terminal prose.\n" +
		"- Use `baha agent describe -o json` to discover the supported machine contract and semantic operations.\n" +
		"- Prefer `baha mcp serve` for agent integration where MCP is available; do not replace BaseHarbor semantics with shell, Docker or Compose execution.\n" +
		"- Use `baha up` or `baha app apply` to converge managed runtime infrastructure.\n" +
		baseHarborAgentsEnd
}

func ensureBaseHarborAgentsSection(root string) (bool, error) {
	path := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read AGENTS.md: %w", err)
	}
	original := string(data)
	startCount := strings.Count(original, baseHarborAgentsStart)
	endCount := strings.Count(original, baseHarborAgentsEnd)
	if startCount != endCount || startCount > 1 {
		return false, errors.New("AGENTS.md contains an ambiguous BaseHarbor managed section; fix the markers before retrying")
	}

	section := baseHarborAgentsSection()
	updated := original
	switch startCount {
	case 0:
		if strings.TrimSpace(updated) != "" {
			updated = strings.TrimRight(updated, "\r\n") + "\n\n"
		}
		updated += section + "\n"
	case 1:
		start := strings.Index(updated, baseHarborAgentsStart)
		endRelative := strings.Index(updated[start:], baseHarborAgentsEnd)
		if start < 0 || endRelative < 0 {
			return false, errors.New("AGENTS.md BaseHarbor section markers are invalid")
		}
		end := start + endRelative + len(baseHarborAgentsEnd)
		updated = updated[:start] + section + updated[end:]
	}

	if updated == original {
		return false, nil
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(updated), mode); err != nil {
		return false, fmt.Errorf("write AGENTS.md: %w", err)
	}
	return true, nil
}

func extractAgentsOption(args []string) ([]string, bool, error) {
	filtered := make([]string, 0, len(args))
	agents := false
	for _, arg := range args {
		if arg == "--agents" {
			if agents {
				return nil, false, usageError("duplicate --agents", "Supply --agents at most once.")
			}
			agents = true
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered, agents, nil
}
