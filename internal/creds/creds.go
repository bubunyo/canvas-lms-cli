// Package creds stores Canvas credentials in the OS keychain, keyed by host.
package creds

import (
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"github.com/zalando/go-keyring"
)

const service = "canvas-cli"

type Kind string

const (
	KindToken  Kind = "token"  // personal access token (Authorization: Bearer)
	KindCookie Kind = "cookie" // browser session cookies (+ X-CSRF-Token on writes)
)

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Credentials struct {
	Kind      Kind      `json:"kind"`
	Token     string    `json:"token,omitempty"`
	TokenID   int64     `json:"token_id,omitempty"` // set when the CLI created the token, so logout can revoke it
	Cookies   []Cookie  `json:"cookies,omitempty"`
	CSRF      string    `json:"csrf,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`
}

var ErrNotFound = errors.New("not logged in")

func account(baseURL string) string {
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return baseURL
}

func Load(baseURL string) (*Credentials, error) {
	s, err := keyring.Get(service, account(baseURL))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func Save(baseURL string, c *Credentials) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return keyring.Set(service, account(baseURL), string(b))
}

func Delete(baseURL string) error {
	err := keyring.Delete(service, account(baseURL))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
