package runtime

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
)

var (
	composeImageProgressRE     = regexp.MustCompile(`(?i)^\\s*(?:image\\s+)?([^\\s]+)\\s+(pulling|pulled)$`)
	composeContainerProgressRE = regexp.MustCompile(`(?i)^\\s*container\\s+([^\\s]+)\\s+(creating|created|starting|started|waiting|healthy|restarting|stopping|stopped)$`)
	composeServiceBuildRE      = regexp.MustCompile(`(?i)^\\s*(?:service\\s+)?([^\\s]+)\\s+(building|built)$`)
)

type composeProgressCapture struct {
	mu         sync.Mutex
	raw        bytes.Buffer
	partial    string
	onProgress func(string)
	last       string
}

func newComposeProgressCapture(onProgress func(string)) *composeProgressCapture {
	return &composeProgressCapture{onProgress: onProgress}
}

func (c *composeProgressCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, _ = c.raw.Write(p)
	text := c.partial + string(p)
	lines := strings.Split(text, "\\n")
	c.partial = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		c.report(line)
	}
	return len(p), nil
}

func (c *composeProgressCapture) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if strings.TrimSpace(c.partial) != "" {
		c.report(c.partial)
		c.partial = ""
	}
}

func (c *composeProgressCapture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.raw.String()
}

func (c *composeProgressCapture) report(line string) {
	if c.onProgress == nil {
		return
	}
	detail := classifyComposeProgressLine(line)
	if detail == "" || detail == c.last {
		return
	}
	c.last = detail
	c.onProgress(detail)
}

func classifyComposeProgressLine(line string) string {
	line = strings.TrimSpace(strings.TrimPrefix(line, "✔"))
	if line == "" {
		return ""
	}
	lower := strings.ToLower(line)

	if match := composeImageProgressRE.FindStringSubmatch(line); len(match) == 3 {
		switch strings.ToLower(match[2]) {
		case "pulling":
			return "pulling image " + safeComposeProgressName(match[1])
		case "pulled":
			return "image ready " + safeComposeProgressName(match[1])
		}
	}
	if match := composeServiceBuildRE.FindStringSubmatch(line); len(match) == 3 {
		switch strings.ToLower(match[2]) {
		case "building":
			return "building image for " + safeComposeProgressName(match[1])
		case "built":
			return "image build complete for " + safeComposeProgressName(match[1])
		}
	}
	if match := composeContainerProgressRE.FindStringSubmatch(line); len(match) == 3 {
		name := safeComposeProgressName(match[1])
		switch strings.ToLower(match[2]) {
		case "creating":
			return "creating container " + name
		case "created":
			return "container created " + name
		case "starting":
			return "starting container " + name
		case "started":
			return "container started " + name
		case "waiting":
			return "waiting for container " + name
		case "healthy":
			return "container healthy " + name
		case "restarting":
			return "restarting container " + name
		case "stopping":
			return "stopping container " + name
		case "stopped":
			return "container stopped " + name
		}
	}

	switch {
	case strings.Contains(lower, "load build definition"):
		return "loading build definition"
	case strings.Contains(lower, "load metadata for"):
		return "resolving build image metadata"
	case strings.Contains(lower, "load .dockerignore"):
		return "loading build context rules"
	case strings.Contains(lower, "load build context"):
		return "loading build context"
	case strings.Contains(lower, "exporting to image"):
		return "exporting built image"
	case strings.Contains(lower, "naming to "):
		return "tagging built image"
	case strings.Contains(lower, "network ") && strings.Contains(lower, " creating"):
		return "creating workload network"
	case strings.Contains(lower, "volume ") && strings.Contains(lower, " creating"):
		return "creating workload volume"
	default:
		return ""
	}
}

func safeComposeProgressName(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 120 {
		value = value[:120]
	}
	return value
}
