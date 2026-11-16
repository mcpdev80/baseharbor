package identityprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// The two GUI aliases can differ from the realm's canonical login origin.
// Keycloak's session-check iframe needs that origin in frame-src. Keep all
// other CSP directives and browser security headers unchanged.
func (a *keycloakAdmin) ensureDeveloperConsoleFrames(ctx context.Context, origins []string) error {
	path := "/admin/realms/" + url.PathEscape(operatorRealm("dev"))
	status, body, err := a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	var realm struct {
		BrowserSecurityHeaders map[string]string `json:"browserSecurityHeaders"`
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &realm) != nil || realm.BrowserSecurityHeaders == nil {
		return errors.New("developer realm browser headers are unavailable")
	}
	current, ok := realm.BrowserSecurityHeaders["contentSecurityPolicy"]
	if !ok {
		return errors.New("developer realm content security policy is unavailable")
	}
	desired := developerConsoleFramePolicy(current, origins)
	if desired == current {
		return nil
	}
	realm.BrowserSecurityHeaders["contentSecurityPolicy"] = desired
	status, _, err = a.do(ctx, http.MethodPut, path, realm)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("configure developer console frame policy: HTTP %d", status)
	}
	status, body, err = a.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &realm) != nil || realm.BrowserSecurityHeaders["contentSecurityPolicy"] != desired {
		return errors.New("developer console frame policy did not converge")
	}
	return nil
}

func developerConsoleFramePolicy(current string, origins []string) string {
	frame := "frame-src 'self'"
	for _, origin := range origins {
		frame += " " + origin
	}
	var directives []string
	replaced := false
	for _, part := range strings.Split(current, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if fields[0] == "frame-src" {
			part = frame
			replaced = true
		}
		directives = append(directives, part)
	}
	if !replaced {
		directives = append(directives, frame)
	}
	return strings.Join(directives, "; ") + ";"
}
