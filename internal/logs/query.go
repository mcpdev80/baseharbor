package logs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func waitLokiReady(ctx context.Context, client *http.Client, endpoint string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/ready", nil)
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			if last == nil {
				last = ctx.Err()
			}
			return fmt.Errorf("Loki readiness: %w", last)
		case <-ticker.C:
		}
	}
}

func VerifyApplication(ctx context.Context, m application.Manifest, services []string) error {
	files, err := ExistingProviderFiles(m)
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for _, service := range services {
		if err := waitForStream(ctx, client, endpoint, m, service); err != nil {
			return err
		}
	}
	return nil
}

func waitForStream(ctx context.Context, client *http.Client, endpoint string, m application.Manifest, service string) error {
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	query := fmt.Sprintf(`{baseharbor_application=%q,baseharbor_environment=%q,baseharbor_service=%q}`, m.Name, m.Environment, service)
	var last error
	for {
		ok, err := queryStream(deadline, client, endpoint, query)
		if err == nil && ok {
			return nil
		}
		if err != nil {
			last = err
		} else {
			last = errors.New("Loki has not ingested a matching workload log stream yet")
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("verify Loki ingestion for %s/%s: %w", m.Name, service, last)
		case <-ticker.C:
		}
	}
}

func queryStream(ctx context.Context, client *http.Client, endpoint, query string) (bool, error) {
	now := time.Now()
	values := url.Values{
		"query": {query},
		"start": {strconv.FormatInt(now.Add(-10*time.Minute).UnixNano(), 10)},
		"end": {strconv.FormatInt(now.Add(time.Minute).UnixNano(), 10)},
		"limit": {"1"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/loki/api/v1/query_range?"+values.Encode(), nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return false, fmt.Errorf("Loki query returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Data struct {
			Result []json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return false, err
	}
	return payload.Status == "success" && len(payload.Data.Result) > 0, nil
}
