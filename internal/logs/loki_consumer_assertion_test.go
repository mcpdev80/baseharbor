package logs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/logs"
	"github.com/mcpdev80/baseharbor/internal/serviceaccess"
)

func assertActualLokiConsumerLine(t *testing.T, ctx context.Context, m application.Manifest, expected string) {
	t.Helper()
	files, err := logs.ExistingProviderFiles(m)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := logs.ProviderEndpoint(files)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := serviceaccess.Resolve(m.Environment, "loki", serviceaccess.AuthenticationMTLS)
	if err != nil {
		t.Fatal(err)
	}
	material, err := serviceaccess.ExistingTLSMaterial(policy, filepath.Join(files.Dir, "service-access", "pki"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := serviceaccess.NewHTTPClientForPolicy(material, policy)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	query := fmt.Sprintf(`{baseharbor_application=%q,baseharbor_environment=%q,baseharbor_service="api"} |= %q`, m.Name, m.Environment, expected)
	values := url.Values{"query": {query}, "start": {strconv.FormatInt(now.Add(-10*time.Minute).UnixNano(), 10)}, "end": {strconv.FormatInt(now.UnixNano(), 10)}, "limit": {"10"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/loki/api/v1/query_range?"+values.Encode(), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = policy.ServerName
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("actual consumer query returned HTTP%d", response.StatusCode)
	}
	var payload struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Values [][]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for _, stream := range payload.Data.Result {
		for _, value := range stream.Values {
			if payload.Status == "success" && len(value) == 2 && strings.Contains(value[1], expected) {
				t.Log("actual isolated workload line received through verified Loki TLS")
				return
			}
		}
	}
	t.Fatal("Loki stream metadata alone does not prove the emitted consumer line")
}
