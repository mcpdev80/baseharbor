package externalprovider

import (
	"errors"
	"net/url"
	"strings"
)

// References enter public registration and impact output. Validate their shape
// before storing them; never interpolate rejected contents into an error.
func validatePublicReference(value string, requireURI bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\r\n\x00") || strings.Contains(value, "-----BEGIN") {
		return errors.New("external provider requires material references, not inline values")
	}
	u, err := url.Parse(value)
	if err != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("external provider references must not contain credentials, query or fragment")
	}
	if requireURI && (u.Scheme == "" || (u.Host == "" && u.Path == "" && u.Opaque == "")) {
		return errors.New("external provider credential reference requires a resource URI")
	}
	return nil
}
