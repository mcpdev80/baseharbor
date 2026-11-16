package stableid

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// ValidationError preserves the validation message and provides a precise next
// action to callers without coupling identity validation to a CLI or transport.
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) NextAction() string {
	return "Use a lowercase UUIDv4: 36 characters with hyphens, version 4 and an RFC 4122 variant (8, 9, a or b)."
}

func NewUUIDv4(label string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate %s identity: %w", label, err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		raw[0:4],
		raw[4:6],
		raw[6:8],
		raw[8:10],
		raw[10:16],
	), nil
}

func ValidateUUIDv4(label, id string) error {
	id = strings.TrimSpace(id)
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return &ValidationError{Message: fmt.Sprintf("%s id %q must be a UUIDv4", label, id)}
	}
	for i, r := range id {
		switch i {
		case 8, 13, 18, 23:
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return &ValidationError{Message: fmt.Sprintf("%s id %q must be a lowercase UUIDv4", label, id)}
		}
	}
	if id[14] != '4' {
		return &ValidationError{Message: fmt.Sprintf("%s id %q must use UUID version 4", label, id)}
	}
	switch id[19] {
	case '8', '9', 'a', 'b':
	default:
		return &ValidationError{Message: fmt.Sprintf("%s id %q has an invalid UUID variant", label, id)}
	}
	return nil
}
