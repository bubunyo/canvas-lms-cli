//go:build darwin

package webview

/*
#cgo CFLAGS: -fobjc-arc -Wno-deprecated-declarations
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#include "login_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

// Cocoa must run on the process's main thread. init runs on it, so pin the
// main goroutine there; Login must therefore be called from the main goroutine.
func init() { runtime.LockOSThread() }

var cookieCh chan string

//export canvasCookies
func canvasCookies(s *C.char) {
	if cookieCh == nil {
		return
	}
	select {
	case cookieCh <- C.GoString(s):
	default: // a newer snapshot arrives within a second
	}
}

// Login opens a native window on baseURL/login/saml and blocks until the
// cookies in it form a session that validate accepts, the user closes the
// window, or timeout elapses. Must be called from the main goroutine.
func Login(ctx context.Context, baseURL string, timeout time.Duration, validate Validate, logf func(string, ...any)) (*Session, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	cookieCh = make(chan string, 1)
	defer func() { cookieCh = nil }()

	var (
		result *Session
		failed error
	)
	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var lastNames, lastChecked string
		deadline := time.After(timeout)
		for {
			select {
			case <-quit:
				return
			case <-ctx.Done():
				failed = ctx.Err()
				C.canvas_login_stop()
				return
			case <-deadline:
				failed = fmt.Errorf("login not completed within %s", timeout)
				C.canvas_login_stop()
				return
			case raw := <-cookieCh:
				s, names := parseCookies(raw)
				if key := strings.Join(names, ","); key != lastNames {
					logf("canvas cookies: [%s]", key)
					lastNames = key
				}
				// Canvas also hands out an anonymous session + CSRF cookie on the
				// login page, so only a session the API accepts counts.
				if s == nil || raw == lastChecked {
					continue
				}
				lastChecked = raw
				vctx, cancel := context.WithTimeout(ctx, 15*time.Second)
				ok, err := validate(vctx, s)
				cancel()
				switch {
				case err != nil:
					logf("checking session: %v", err)
				case ok:
					logf("session accepted by API")
					result = s
					C.canvas_login_stop()
					return
				default:
					logf("session not logged in yet")
				}
			}
		}
	}()

	start := C.CString(baseURL + "/login/saml")
	host := C.CString(u.Hostname())
	title := C.CString("Canvas sign-in — " + u.Hostname())
	defer C.free(unsafe.Pointer(start))
	defer C.free(unsafe.Pointer(host))
	defer C.free(unsafe.Pointer(title))

	C.canvas_login_run(start, host, title) // blocks until stopped or window closed
	close(quit)
	<-done

	switch {
	case result != nil:
		return result, nil
	case failed != nil:
		return nil, failed
	default:
		return nil, errors.New("login window was closed before sign-in completed")
	}
}
