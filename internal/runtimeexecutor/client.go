package runtimeexecutor

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/runtimeoperation"
)

type ClientConfig struct {
	URL      string
	CAFile   string
	CertFile string
	KeyFile  string
}

type Client struct {
	endpoint string
	http     *http.Client
}

func NewClient(cfg ClientConfig) (*Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if endpoint == "" {
		return nil, errors.New("runtime executor URL is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, errors.New("runtime executor URL must be absolute HTTPS")
	}
	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read runtime executor CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("runtime executor CA is invalid")
	}
	certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load runtime executor client identity: %w", err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs: roots,
		Certificates: []tls.Certificate{certificate},
		ServerName: parsed.Hostname(),
	}}
	return &Client{
		endpoint: endpoint,
		http: &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) Execute(ctx context.Context, request runtimeoperation.Request) (runtimeoperation.Result, error) {
	response, err := c.execute(ctx, ExecuteRequest{
		Capability: request.Capability,
		Operation: request.Operation,
		Name: request.ResourceName,
	})
	if err != nil {
		return runtimeoperation.Result{}, err
	}
	result := runtimeoperation.Result{ResourceID: response.ResourceID}
	if response.Binding != nil {
		data, err := json.Marshal(response.Binding)
		if err != nil {
			return runtimeoperation.Result{}, err
		}
		if err := json.Unmarshal(data, &result.Binding); err != nil {
			return runtimeoperation.Result{}, err
		}
	}
	return result, nil
}

func (c *Client) Resolve(ctx context.Context, capability, operation, name string) (ExecuteResponse, error) {
	return c.execute(ctx, ExecuteRequest{Capability: capability, Operation: operation, Name: name})
}

func (c *Client) execute(ctx context.Context, request ExecuteRequest) (ExecuteResponse, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return ExecuteResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/internal/v1/execute", bytes.NewReader(payload))
	if err != nil {
		return ExecuteResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return ExecuteResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ExecuteResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return ExecuteResponse{}, fmt.Errorf("runtime executor request failed with HTTP %d", resp.StatusCode)
	}
	var response ExecuteResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return ExecuteResponse{}, errors.New("decode runtime executor response")
	}
	return response, nil
}
