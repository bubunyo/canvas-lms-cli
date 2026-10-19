// Package webview signs in to Canvas through a native login window, so the
// institution's SSO (Canvas → Instructure Identity → SAML IdP + MFA) runs in a
// real browser engine, and returns the resulting Canvas session cookies.
package webview

import (
	"context"
	"net/url"
	"strings"

	"github.com/bubunyo/canvas-lms-cli/internal/creds"
)

type Session struct {
	Cookies []creds.Cookie
	CSRF    string
}

// Validate reports whether a candidate session is logged in (e.g. by calling
// /api/v1/users/self). It returns false, nil while the user is still signing in.
type Validate func(ctx context.Context, s *Session) (bool, error)

// parseCookies turns "name\tvalue\n" lines into a Session. It returns nil until
// both a Canvas session cookie and the CSRF cookie are present.
func parseCookies(raw string) (*Session, []string) {
	s := &Session{}
	var names []string
	var hasSession bool
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		name, value, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		names = append(names, name)
		s.Cookies = append(s.Cookies, creds.Cookie{Name: name, Value: value})
		switch name {
		case "_normandy_session", "canvas_session", "_legacy_normandy_session":
			hasSession = true
		case "_csrf_token":
			if v, err := url.QueryUnescape(value); err == nil {
				s.CSRF = v
			}
		}
	}
	if !hasSession || s.CSRF == "" {
		return nil, names
	}
	return s, names
}
