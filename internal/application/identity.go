package application

import (
	"crypto/rand"
	"fmt"
	"strings"
)

func NewApplicationID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate application identity: %w", err)
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

func MustNewApplicationID() string {
	id, err := NewApplicationID()
	if err != nil {
		panic(err)
	}
	return id
}

func ValidateApplicationID(id string) error {
	id = strings.TrimSpace(id)
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return fmt.Errorf("application id %q must be a UUIDv4", id)
	}
	for i, r := range id {
		switch i {
		case 8, 13, 18, 23:
			continue
		}
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return fmt.Errorf("application id %q must be a lowercase UUIDv4", id)
		}
	}
	if id[14] != '4' {
		return fmt.Errorf("application id %q must use UUID version 4", id)
	}
	switch id[19] {
	case '8', '9', 'a', 'b':
	default:
		return fmt.Errorf("application id %q has an invalid UUID variant", id)
	}
	return nil
}
