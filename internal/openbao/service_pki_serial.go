package openbao

import (
	"encoding/hex"
	"errors"
	"strings"
)

// OpenBao stores PKI certificate serials as separated hex bytes. X509 callers
// commonly have an unseparated positive integer instead. Normalize at the
// issuer boundary so revocation looks up the actual stored certificate.
func canonicalServiceRevocationSerial(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	invalid := errors.New("service certificate serial must be a bounded positive hexadecimal integer")
	if strings.ContainsAny(value, "\r\n") {
		return "", invalid
	}
	separator := ""
	if strings.Contains(value, ":") {
		separator = ":"
	}
	if strings.Contains(value, "-") {
		if separator != "" {
			return "", invalid
		}
		separator = "-"
	}
	if separator != "" {
		for _, part := range strings.Split(value, separator) {
			if len(part) != 2 {
				return "", invalid
			}
		}
		value = strings.ReplaceAll(value, separator, "")
	}
	if len(value) == 0 || len(value) > 128 {
		return "", invalid
	}
	if len(value)%2 != 0 {
		value = "0" + value
	}
	data, err := hex.DecodeString(value)
	if err != nil {
		return "", invalid
	}
	for len(data) > 0 && data[0] == 0 {
		data = data[1:]
	}
	if len(data) == 0 {
		return "", invalid
	}
	parts := make([]string, len(data))
	for i, b := range data {
		parts[i] = hex.EncodeToString([]byte{b})
	}
	return strings.Join(parts, ":"), nil
}
