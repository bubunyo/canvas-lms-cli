//go:build !darwin

package webview

import (
	"context"
	"errors"
	"time"
)

func Login(context.Context, string, time.Duration, Validate, func(string, ...any)) (*Session, error) {
	return nil, errors.New("the built-in login window is only available on macOS; use `canvas-cli auth login --token`")
}
