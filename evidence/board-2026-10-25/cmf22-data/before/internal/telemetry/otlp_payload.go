package telemetry

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/observability"
	"github.com/mcpdev80/baseharbor/internal/providertopology"
)

func providerInteractionTracePayload(m application.Manifest, source observability.SignalSource) ([]byte, string) {
	now := uint64(time.Now().UnixNano())
	identity := m.Name + "\x00" + m.Environment + "\x00" + string(source.Provider) + "\x00" + source.ID + "\x00" + strconv.FormatUint(now, 10)
	traceHash := sha256.Sum256([]byte("trace\x00" + identity))
	spanHash := sha256.Sum256([]byte("span\x00" + identity))
	traceID := append([]byte(nil), traceHash[:16]...)
	spanID := append([]byte(nil), spanHash[:8]...)

	span := appendBytes(nil, 1, traceID)
	span = appendBytes(span, 2, spanID)
	span = appendString(span, 5, "baseharbor.provider.interaction.verify")
	span = appendFixed64(span, 7, now)
	span = appendFixed64(span, 8, now+1)
	span = appendMessage(span, 9, keyValue("baseharbor.provider", string(source.Provider)))
	span = appendMessage(span, 9, keyValue("baseharbor.source", source.ID))
	span = appendMessage(span, 9, keyValue("baseharbor.source_class", string(source.Class)))
	span = appendMessage(span, 9, keyValue("baseharbor.resource", source.Target))
	if source.SemanticConvention != "" {
		span = appendMessage(span, 9, keyValue("baseharbor.semantic_convention", source.SemanticConvention))
	}

	scopeSpans := appendMessage(nil, 2, span)
	resource := []byte{}
	resource = appendMessage(resource, 1, keyValue("service.name", "baseharbor"))
	resource = appendMessage(resource, 1, keyValue("service.namespace", m.Name))
	resource = appendMessage(resource, 1, keyValue("deployment.environment.name", m.Environment))
	resource = appendMessage(resource, 1, keyValue("baseharbor.application", m.Name))
	resourceSpans := appendMessage(nil, 1, resource)
	resourceSpans = appendMessage(resourceSpans, 2, scopeSpans)
	return appendMessage(nil, 1, resourceSpans), hex.EncodeToString(traceID)
}

func VerificationTracePayload(m application.Manifest) []byte {
	return probeTracePayload(m)
}

func probeTracePayload(m application.Manifest) []byte {
	now := uint64(time.Now().UnixNano())
	traceID := []byte{0x42, 0x61, 0x73, 0x65, 0x48, 0x61, 0x72, 0x62, 0x6f, 0x72, 0x30, 0x34, 0x30, 0x37, 0x00, 0x01}
	spanID := []byte{0x42, 0x48, 0x30, 0x34, 0x30, 0x37, 0x00, 0x01}
	span := appendBytes(nil, 1, traceID)
	span = appendBytes(span, 2, spanID)
	span = appendString(span, 5, "baseharbor.otlp.verify")
	span = appendFixed64(span, 7, now)
	span = appendFixed64(span, 8, now+1)
	scopeSpans := appendMessage(nil, 2, span)
	resource := []byte{}
	resource = appendMessage(resource, 1, keyValue("service.name", m.Name))
	resource = appendMessage(resource, 1, keyValue("service.namespace", m.Name))
	resource = appendMessage(resource, 1, keyValue("deployment.environment.name", m.Environment))
	resource = appendMessage(resource, 1, keyValue("baseharbor.application", m.Name))
	resourceSpans := appendMessage(nil, 1, resource)
	resourceSpans = appendMessage(resourceSpans, 2, scopeSpans)
	return appendMessage(nil, 1, resourceSpans)
}

func keyValue(key, value string) []byte {
	any := appendString(nil, 1, value)
	msg := appendString(nil, 1, key)
	return appendMessage(msg, 2, any)
}

func appendTag(dst []byte, field int, wire byte) []byte {
	return binary.AppendUvarint(dst, uint64(field<<3)|uint64(wire))
}
func appendMessage(dst []byte, field int, msg []byte) []byte {
	dst = appendTag(dst, field, 2)
	dst = binary.AppendUvarint(dst, uint64(len(msg)))
	return append(dst, msg...)
}
func appendBytes(dst []byte, field int, value []byte) []byte {
	return appendMessage(dst, field, value)
}
func appendString(dst []byte, field int, value string) []byte {
	return appendMessage(dst, field, []byte(value))
}
func appendFixed64(dst []byte, field int, value uint64) []byte {
	dst = appendTag(dst, field, 1)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], value)
	return append(dst, buf[:]...)
}

func collectorUpstreams(members int) []string {
	var upstreams []string
	for _, name := range providertopology.Names("otel-collector", members) {
		upstreams = append(upstreams, "https://"+name+":4318")
	}
	return upstreams
}
