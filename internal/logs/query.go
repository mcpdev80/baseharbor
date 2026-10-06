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
	"github.com/mcpdev80/baseharbor/internal/observability"
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
	return VerifyApplicationAt(ctx, m, services, "", "")
}

func VerifyApplicationAt(ctx context.Context, m application.Manifest, services []string, dataDir, namespace string) error {
	var files ProviderFiles
	var err error
	if strings.TrimSpace(dataDir) == "" {
		files, err = ExistingProviderFiles(m)
	} else {
		files, err = ExistingProviderFilesAt(dataDir, namespace, m)
	}
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	client, err := lokiHTTPClient(m, files)
	if err != nil {
		return err
	}
	for _, service := range services {
		if err := waitForStream(ctx, client, endpoint, m, service); err != nil {
			return err
		}
	}
	return nil
}

func VerifyProviderSources(ctx context.Context, m application.Manifest, sources []observability.SignalSource) error {
	return VerifyProviderSourcesAt(ctx, m, sources, "", "")
}

func VerifyProviderSourcesAt(ctx context.Context, m application.Manifest, sources []observability.SignalSource, dataDir, namespace string) error {
	if len(sources) == 0 {
		return nil
	}
	for _, source := range sources {
		if err := validateProviderLogSource(m, source); err != nil {
			return err
		}
	}
	var files ProviderFiles
	var err error
	if strings.TrimSpace(dataDir) == "" {
		files, err = ExistingProviderFiles(m)
	} else {
		files, err = ExistingProviderFilesAt(dataDir, namespace, m)
	}
	if err != nil {
		return err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return err
	}
	client, err := lokiHTTPClient(m, files)
	if err != nil {
		return err
	}
	for _, source := range sources {
		_, service, _ := observability.ParseRuntimeTarget(source.Target)
		var query string
		switch source.Class {
		case observability.SourceApplicationProvider:
			query = fmt.Sprintf(
				`{baseharbor_application=%q,baseharbor_environment=%q,baseharbor_source_class="application-provider",baseharbor_provider=%q,baseharbor_service=%q}`,
				m.Name,
				m.Environment,
				string(source.Provider),
				service,
			)
		case observability.SourcePlatformProvider:
			query = fmt.Sprintf(
				`{baseharbor_source_class="platform-provider",baseharbor_provider=%q,baseharbor_service=%q}`,
				string(source.Provider),
				service,
			)
		default:
			continue
		}
		if err := waitForSeries(ctx, client, endpoint, query, "provider "+string(source.Provider)+"/"+service); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderLogSource(m application.Manifest, source observability.SignalSource) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("provider log source %q: %w", source.ID, err)
	}
	if source.Kind != observability.SignalLogs {
		return fmt.Errorf("provider log source %q has signal kind %q, want logs", source.ID, source.Kind)
	}
	if source.Class != observability.SourceApplicationProvider && source.Class != observability.SourcePlatformProvider {
		return fmt.Errorf("provider log source %q has unsupported source class %q", source.ID, source.Class)
	}
	if source.Class == observability.SourceApplicationProvider && strings.TrimSpace(source.OwnerApplication) != strings.TrimSpace(m.Name) {
		return fmt.Errorf("provider log source %q belongs to application %q, want %q", source.ID, source.OwnerApplication, m.Name)
	}
	if _, _, ok := observability.ParseRuntimeTarget(source.Target); !ok {
		return fmt.Errorf("provider log source %q has invalid runtime target %q", source.ID, source.Target)
	}
	return nil
}

func waitForStream(ctx context.Context, client *http.Client, endpoint string, m application.Manifest, service string) error {
	query := fmt.Sprintf(`{baseharbor_application=%q,baseharbor_environment=%q,baseharbor_service=%q}`, m.Name, m.Environment, service)
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var last error
	for {
		probeCtx, probeCancel := context.WithTimeout(deadline, 20*time.Second)
		ok, queryErr := queryStream(probeCtx, client, endpoint, query)
		probeCancel()
		if queryErr == nil && ok {
			return nil
		}

		probeCtx, probeCancel = context.WithTimeout(deadline, 20*time.Second)
		ok, seriesErr := querySeries(probeCtx, client, endpoint, query)
		probeCancel()
		if seriesErr == nil && ok {
			return nil
		}

		switch {
		case queryErr != nil && seriesErr != nil:
			last = fmt.Errorf("query_range: %v; series: %v", queryErr, seriesErr)
		case queryErr != nil:
			last = queryErr
		case seriesErr != nil:
			last = seriesErr
		default:
			last = errors.New("Loki has not ingested a matching log stream yet")
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("verify Loki ingestion for %s/%s: %w", m.Name, service, last)
		case <-ticker.C:
		}
	}
}

func waitForSeries(ctx context.Context, client *http.Client, endpoint, match, description string) error {
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last error
	for {
		probeCtx, probeCancel := context.WithTimeout(deadline, 3*time.Second)
		ok, err := querySeries(probeCtx, client, endpoint, match)
		probeCancel()
		if err == nil && ok {
			return nil
		}
		if err != nil {
			last = err
		} else {
			last = errors.New("Loki has not ingested a matching log stream yet")
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("verify Loki ingestion for %s: %w", description, last)
		case <-ticker.C:
		}
	}
}

func querySeries(ctx context.Context, client *http.Client, endpoint, match string) (bool, error) {
	now := time.Now()
	values := url.Values{
		"match[]": {match},
		"start":   {strconv.FormatInt(now.Add(-10*time.Minute).UnixNano(), 10)},
		"end":     {strconv.FormatInt(now.UnixNano(), 10)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/loki/api/v1/series?"+values.Encode(), nil)
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
		return false, fmt.Errorf("Loki series query returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Status string              `json:"status"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return false, err
	}
	return payload.Status == "success" && len(payload.Data) > 0, nil
}

func waitForQuery(ctx context.Context, client *http.Client, endpoint, query, description string) error {
	deadline, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var last error
	for {
		probeCtx, probeCancel := context.WithTimeout(deadline, 3*time.Second)
		ok, err := queryStream(probeCtx, client, endpoint, query)
		probeCancel()
		if err == nil && ok {
			return nil
		}
		if err != nil {
			last = err
		} else {
			last = errors.New("Loki has not ingested a matching log stream yet")
		}
		select {
		case <-deadline.Done():
			return fmt.Errorf("verify Loki ingestion for %s: %w", description, last)
		case <-ticker.C:
		}
	}
}

func queryStream(ctx context.Context, client *http.Client, endpoint, query string) (bool, error) {
	now := time.Now()
	values := url.Values{
		"query": {query},
		"start": {strconv.FormatInt(now.Add(-10*time.Minute).UnixNano(), 10)},
		"end":   {strconv.FormatInt(now.UnixNano(), 10)},
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
		Data   struct {
			Result []json.RawMessage `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return false, err
	}
	return payload.Status == "success" && len(payload.Data.Result) > 0, nil
}
