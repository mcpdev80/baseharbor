package application

import (
	"fmt"
	"strconv"
	"strings"
)

// This file holds the services-section field grammar extracted from
// manifest_parse.go for the source-readability line limit. Behaviour is
// unchanged.

func (p *manifestYAMLParser) parseServiceField(lineNo int, trim string) error {
	if trim == "instances:" && (p.service == "sql" || p.service == "cache" || p.service == "key_value" || p.service == "document_database" || p.service == "messaging_queue" || p.service == "messaging_pubsub" || p.service == "messaging_stream") {
		p.serviceField = "instances"
		return nil
	}
	if trim == "buckets:" && p.service == "object_storage" {
		p.serviceField = "buckets"
		return nil
	}

	key, value, ok := strings.Cut(trim, ":")
	if !ok {
		if key == "perses" && p.service == "observability" {
			enabled, err := strconv.ParseBool(strings.TrimSpace(value))
			if err != nil {
				return fmt.Errorf("line %d: invalid perses value", lineNo)
			}
			p.manifest.Services.Perses = enabled
			return nil
		}
		return fmt.Errorf("line %d: expected enabled: true|false or management_ui: true|false or instances:", lineNo)
	}
	if key == "perses" {
		if p.service != "observability" {
			return fmt.Errorf("line %d: perses is only valid under observability", lineNo)
		}
		enabled, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("line %d: invalid perses value", lineNo)
		}
		p.manifest.Services.Perses = enabled
		return nil
	}
	if key != "enabled" && key != "management_ui" {
		return fmt.Errorf("line %d: expected enabled: true|false, management_ui: true|false or instances:", lineNo)
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("line %d: invalid enabled value", lineNo)
	}
	if key == "management_ui" {
		switch p.service {
		case "sql":
			p.manifest.Services.SQLManagementUI = enabled
		case "cache":
			p.manifest.Services.CacheManagementUI = enabled
		case "key_value":
			p.manifest.Services.KeyValueManagementUI = enabled
		case "document_database":
			p.manifest.Services.DocumentDatabaseManagementUI = enabled
		case "messaging_queue", "messaging_pubsub", "messaging_stream":
			p.manifest.Services.MessagingManagementUI = enabled
		case "object_storage":
			p.manifest.Services.ObjectStorageManagementUI = enabled
		case "secrets":
			p.manifest.Services.SecretsManagementUI = enabled
		case "identity":
			p.manifest.Services.IdentityManagementUI = enabled
		case "observability":
			p.manifest.Services.ObservabilityManagementUI = enabled
		}
		return nil
	}
	switch p.service {
	case "sql":
		p.manifest.Services.SQL = enabled
	case "cache":
		p.manifest.Services.Cache = enabled
	case "key_value":
		p.manifest.Services.KeyValue = enabled
	case "document_database":
		p.manifest.Services.DocumentDatabase = enabled
	case "messaging_queue":
		p.manifest.Services.MessagingQueue = enabled
	case "messaging_pubsub":
		p.manifest.Services.MessagingPubSub = enabled
	case "messaging_stream":
		p.manifest.Services.MessagingStream = enabled
	case "object_storage":
		p.manifest.Services.ObjectStorage = enabled
	case "secrets":
		p.manifest.Services.Secrets = enabled
	case "identity":
		p.manifest.Services.Identity = enabled
	case "observability":
		if enabled {
			return fmt.Errorf("line %d: observability is selected by telemetry/metrics/logs/traces; only management_ui is valid here", lineNo)
		}
	}
	return nil
}
