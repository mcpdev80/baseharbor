package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/sourcetransfer"
)

// uploadLocalSource records the exact bytes currently on the developer's
// machine. It carries neither repository credentials nor an unverified branch
// claim. A verified Core TLS/OIDC session is mandatory for every HTTP request.
func (c *verifiedRemoteCore) uploadLocalSource(ctx context.Context, target, name, environment, root string) (string, error) {
	if c == nil || c.client == nil || c.origin == "" || c.token == "" || c.target != target || c.tenant == "" {
		return "", errors.New("source transfer requires a verified management Core connection")
	}
	endpoint := c.origin + "/api/v1/machine/source-transfers"
	currentURL := endpoint + "?target=" + url.QueryEscape(target) + "&application=" + url.QueryEscape(name) + "&environment=" + url.QueryEscape(environment)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, currentURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("read current source snapshot: %w", err)
	}
	var base string
	switch resp.StatusCode {
	case http.StatusNotFound:
	case http.StatusOK:
		var response struct {
			Digest string `json:"digest"`
		}
		decoder := json.NewDecoder(io.LimitReader(resp.Body, 2048))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&response) != nil || decoder.Decode(new(any)) != io.EOF {
			resp.Body.Close()
			return "", errors.New("current snapshot response is invalid")
		}
		base = response.Digest
	default:
		status := resp.StatusCode
		resp.Body.Close()
		return "", fmt.Errorf("Core refused source snapshot discovery (HTTP %d)", status)
	}
	resp.Body.Close()
	manifest, bytesByPath, err := sourcetransfer.Capture(ctx, root, target, name, environment, base)
	if err != nil {
		return "", fmt.Errorf("capture developer source: %w", err)
	}
	if base == manifest.Digest {
		return base, nil // exact unchanged local snapshot: no new upload required
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err = c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("begin source upload: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		status := resp.StatusCode
		resp.Body.Close()
		return "", fmt.Errorf("Core refused source upload (HTTP %d)", status)
	}
	var begin struct {
		ID      string   `json:"id"`
		Missing []string `json:"missing"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&begin) != nil || decoder.Decode(new(any)) != io.EOF {
		resp.Body.Close()
		return "", errors.New("source upload admission is invalid")
	}
	resp.Body.Close()
	if len(begin.ID) != 32 {
		return "", errors.New("source upload identifier is invalid")
	}
	selected := map[string]bool{}
	for _, f := range manifest.Files {
		selected[f.Path] = true
	}
	for _, p := range begin.Missing {
		if _, ok := selected[p]; !ok {
			return "", errors.New("Core requested an unselected source path")
		}
		if !selected[p] {
			return "", errors.New("source transfer returned duplicate path")
		}
		selected[p] = false
		data := bytesByPath[p]
		for offset := 0; offset < len(data) || (len(data) == 0 && offset == 0); {
			end := offset + sourcetransfer.MaxChunkBytes
			if end > len(data) {
				end = len(data)
			}
			payload, err := json.Marshal(struct {
				Path   string `json:"path"`
				Offset int64  `json:"offset"`
				Data   []byte `json:"data"`
			}{p, int64(offset), data[offset:end]})
			if err != nil {
				return "", err
			}
			chunkURL := endpoint + "/" + begin.ID
			request, err := http.NewRequestWithContext(ctx, http.MethodPut, chunkURL, bytes.NewReader(payload))
			if err != nil {
				return "", err
			}
			request.Header.Set("Authorization", "Bearer "+c.token)
			request.Header.Set("Content-Type", "application/json")
			reply, err := c.client.Do(request)
			if err != nil {
				return "", fmt.Errorf("send source chunk %q: %w", p, err)
			}
			if reply.StatusCode != http.StatusOK {
				code := reply.StatusCode
				reply.Body.Close()
				return "", fmt.Errorf("Core refused source chunk %q (HTTP %d)", p, code)
			}
			io.Copy(io.Discard, io.LimitReader(reply.Body, 4096))
			reply.Body.Close()
			if len(data) == 0 {
				break
			}
			offset = end
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/"+begin.ID+"/commit", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	reply, err := c.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("commit source snapshot: %w", err)
	}
	defer reply.Body.Close()
	if reply.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Core refused source commit (HTTP %d)", reply.StatusCode)
	}
	var confirmed struct {
		Digest string `json:"digest"`
	}
	decoder = json.NewDecoder(io.LimitReader(reply.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&confirmed) != nil || decoder.Decode(new(any)) != io.EOF || confirmed.Digest != manifest.Digest {
		return "", errors.New("Core source receipt does not match the captured bytes")
	}
	return confirmed.Digest, nil
}

// remoteLocalSourceRoot selects source bytes only for the exact application in
// the current checkout. An explicit NAME for another registered app never
// authorizes replacing it with unrelated local files.
func remoteLocalSourceRoot(name, environment string) (string, error) {
	path, err := application.FindRepositoryManifest(".")
	if errors.Is(err, application.ErrRepositoryManifestNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	manifest, err := application.LoadManifestFile(path)
	if err != nil {
		return "", err
	}
	if manifest.Name != name || manifest.Environment != environment {
		return "", nil
	}
	root, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("developer repository root must be a real directory")
	}
	return root, nil
}
func remotePayloadWithSnapshot(payload json.RawMessage, digest string) (json.RawMessage, error) {
	if strings.TrimSpace(digest) == "" {
		return payload, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil || fields == nil {
		return nil, errors.New("remote apply payload is invalid")
	}
	if _, has := fields["source_snapshot_digest"]; has {
		return nil, errors.New("duplicate source snapshot selection")
	}
	value, err := json.Marshal(digest)
	if err != nil {
		return nil, err
	}
	fields["source_snapshot_digest"] = value
	return json.Marshal(fields)
}