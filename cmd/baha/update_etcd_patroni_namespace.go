package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// verifyRestoredPatroniNamespace verifies durable Patroni initialization and
// configuration. It deliberately makes no claim about a live Patroni leader.
func verifyRestoredPatroniNamespace(data []byte, prefix string) error {
	if !strings.HasPrefix(prefix, "/service/") || !strings.HasSuffix(prefix, "/") {
		return errors.New("invalid Patroni namespace")
	}
	var response struct {
		KVs []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
			Lease int64  `json:"lease"`
		} `json:"kvs"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	found := map[string]bool{}
	for _, kv := range response.KVs {
		key, err := base64.StdEncoding.DecodeString(kv.Key)
		if err != nil || !strings.HasPrefix(string(key), prefix) {
			return errors.New("restored Patroni namespace contains invalid or foreign key")
		}
		if found[string(key)] {
			return errors.New("duplicate restored Patroni key")
		}
		found[string(key)] = true
		if string(key) != prefix+"initialize" && string(key) != prefix+"config" {
			continue
		}
		value, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil || kv.Lease != 0 {
			return errors.New("invalid or leased durable Patroni state")
		}
		if string(key) == prefix+"initialize" {
			id, err := strconv.ParseUint(string(value), 10, 64)
			if err != nil || id == 0 {
				return errors.New("restored Patroni PostgreSQL system identity invalid")
			}
		} else {
			var config map[string]json.RawMessage
			if err := json.Unmarshal(value, &config); err != nil || len(config) == 0 {
				return errors.New("restored Patroni dynamic configuration invalid")
			}
		}
	}
	if !found[prefix+"initialize"] || !found[prefix+"config"] {
		return errors.New("restored etcd quorum lacks durable Patroni initialization/configuration")
	}
	return nil
}
