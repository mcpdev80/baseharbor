package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
)

const (
	recoveryQueryBatch = 5000
	recoveryMaxEntries = 200000
	recoveryMaxBytes   = 256 << 20
)

type HistoryEntry struct {
	Timestamp string `json:"timestamp"`
	Line      string `json:"line"`
}

type HistoryStream struct {
	Labels  map[string]string `json:"labels"`
	Entries []HistoryEntry    `json:"entries"`
}

type HistoryBackup struct {
	Streams []HistoryStream `json:"streams"`
}

func ExportApplicationHistoryAt(ctx context.Context, m application.Manifest, dataDir, namespace string) (HistoryBackup, error) {
	files, err := ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {
		return HistoryBackup{}, err
	}
	endpoint, err := ProviderEndpoint(files)
	if err != nil {
		return HistoryBackup{}, err
	}
	client, err := lokiHTTPClient(m, files)
	if err != nil {
		return HistoryBackup{}, err
	}
	query := fmt.Sprintf(`{baseharbor_application=%q,baseharbor_environment=%q}`, m.Name, m.Environment)
	start := int64(0)
	end := time.Now().Add(time.Minute).UnixNano()
	streams := map[string]*HistoryStream{}
	totalEntries := 0
	totalBytes := 0
	for {
		values := url.Values{
			"query":     {query},
			"start":     {strconv.FormatInt(start, 10)},
			"end":       {strconv.FormatInt(end, 10)},
			"limit":     {strconv.Itoa(recoveryQueryBatch)},
			"direction": {"forward"},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/loki/api/v1/query_range?"+values.Encode(), nil)
		if err != nil {
			return HistoryBackup{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return HistoryBackup{}, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, recoveryMaxBytes+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return HistoryBackup{}, readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return HistoryBackup{}, fmt.Errorf("Loki recovery query returned HTTP %d", resp.StatusCode)
		}
		if len(body) > recoveryMaxBytes {
			return HistoryBackup{}, errors.New("Loki recovery response exceeds size limit")
		}
		var payload struct {
			Status string `json:"status"`
			Data struct {
				Result []struct {
					Stream map[string]string `json:"stream"`
					Values [][]string        `json:"values"`
				} `json:"result"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return HistoryBackup{}, fmt.Errorf("decode Loki recovery query: %w", err)
		}
		if payload.Status != "success" {
			return HistoryBackup{}, errors.New("Loki recovery query was not successful")
		}
		batchEntries := 0
		lastTimestamp := int64(-1)
		for _, result := range payload.Data.Result {
			if result.Stream["baseharbor_application"] != m.Name || result.Stream["baseharbor_environment"] != m.Environment {
				return HistoryBackup{}, errors.New("Loki recovery query returned data outside the application scope")
			}
			keyData, _ := json.Marshal(result.Stream)
			key := string(keyData)
			stream := streams[key]
			if stream == nil {
				labels := make(map[string]string, len(result.Stream))
				for k, v := range result.Stream {\n\t\t\t\t\tlabels[k] = v\n\t\t\t\t}
				stream = &HistoryStream{Labels: labels}
				streams[key] = stream
			}
			for _, value := range result.Values {
				if len(value) != 2 {
					return HistoryBackup{}, errors.New("Loki recovery value is malformed")
				}
				ts, err := strconv.ParseInt(value[0], 10, 64)
				if err != nil {
					return HistoryBackup{}, errors.New("Loki recovery timestamp is malformed")
				}
				if ts > lastTimestamp {\n\t\t\t\t\tlastTimestamp = ts\n\t\t\t\t}
				stream.Entries = append(stream.Entries, HistoryEntry{Timestamp: value[0], Line: value[1]})
				totalEntries++
				batchEntries++
				totalBytes += len(value[0]) + len(value[1])
				if totalEntries > recoveryMaxEntries || totalBytes > recoveryMaxBytes {
					return HistoryBackup{}, errors.New("Loki application history exceeds recovery limits")
				}
			}
		}
		if batchEntries < recoveryQueryBatch {
			break
		}
		if lastTimestamp < start {
			return HistoryBackup{}, errors.New("Loki recovery pagination did not advance")
		}
		start = lastTimestamp + 1
		if start > end {\n\t\t\tbreak\n\t\t}
	}
	keys := make([]string, 0, len(streams))
	for key := range streams {\n\t\tkeys = append(keys, key)\n\t}
	sort.Strings(keys)
	backup := HistoryBackup{Streams: make([]HistoryStream, 0, len(keys))}
	for _, key := range keys {
		stream := streams[key]
		sort.SliceStable(stream.Entries, func(i, j int) bool { return stream.Entries[i].Timestamp < stream.Entries[j].Timestamp })
		backup.Streams = append(backup.Streams, *stream)
	}
	return backup, nil
}

func RestoreApplicationHistoryAt(ctx context.Context, m application.Manifest, dataDir, namespace string, backup HistoryBackup) error {
	files, err := ExistingProviderFilesAt(dataDir, namespace, m)
	if err != nil {\n\t\treturn err\n\t}
	endpoint, err := ProviderEndpoint(files)
	if err != nil { return err }
	client, err := lokiHTTPClient(m, files)
	if err != nil { return err }
	for _, stream := range backup.Streams {
		if stream.Labels["baseharbor_application"] != m.Name || stream.Labels["baseharbor_environment"] != m.Environment {
			return errors.New("refusing to restore Loki history outside the application scope")
		}
		for offset := 0; offset < len(stream.Entries); offset += 1000 {
			end := offset + 1000
			if end > len(stream.Entries) {\n\t\t\t\tend = len(stream.Entries)\n\t\t\t}
			values := make([][]string, 0, end-offset)
			for _, entry := range stream.Entries[offset:end] {
				if _, err := strconv.ParseInt(entry.Timestamp, 10, 64); err != nil {
					return errors.New("Loki recovery timestamp is malformed")
				}
				values = append(values, []string{entry.Timestamp, entry.Line})
			}
			payload := struct {
				Streams []struct {
					Stream map[string]string `json:"stream"`
					Values [][]string        `json:"values"`
				} `json:"streams"`
			}{}
			item := struct {
				Stream map[string]string `json:"stream"`
				Values [][]string        `json:"values"`
			}{Stream: stream.Labels, Values: values}
			payload.Streams = append(payload.Streams, item)
			body, err := json.Marshal(payload)
			if err != nil { return err }
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/loki/api/v1/push", bytes.NewReader(body))
			if err != nil { return err }
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil { return err }
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("Loki recovery push returned HTTP %d", resp.StatusCode)
			}
		}
	}
	return nil
}
