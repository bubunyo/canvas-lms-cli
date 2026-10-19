// Package canvas is a minimal Canvas LMS REST API client.
package canvas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bubunyo/canvas-lms-cli/internal/creds"
)

var ErrUnauthorized = errors.New("not authenticated (session expired or token revoked): run `canvas-cli auth login`")

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("canvas: HTTP %d: %s", e.Status, e.Message) }

type Client struct {
	BaseURL string
	creds   *creds.Credentials
	http    *http.Client
}

func New(baseURL string, c *creds.Credentials) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		creds:   c,
		http: &http.Client{
			Timeout: 60 * time.Second,
			// An expired cookie session answers API calls with a redirect to /login;
			// surface that instead of silently following it.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Client) url(path string, q url.Values) string {
	u := path
	if !strings.HasPrefix(path, "http") {
		u = c.BaseURL + path
	}
	if len(q) > 0 {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + q.Encode()
	}
	return u
}

func (c *Client) do(ctx context.Context, method, rawURL string, body any) (*http.Response, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "canvas-cli")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		c.authorize(req)

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusForbidden && attempt < 4 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if strings.Contains(string(b), "Rate Limit Exceeded") {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
				continue
			}
			return nil, apiError(resp.StatusCode, b)
		}
		if err := checkStatus(resp); err != nil {
			return nil, err
		}
		throttle(resp)
		return resp, nil
	}
}

func (c *Client) authorize(req *http.Request) {
	if c.creds == nil {
		return
	}
	switch c.creds.Kind {
	case creds.KindToken:
		req.Header.Set("Authorization", "Bearer "+c.creds.Token)
	case creds.KindCookie:
		for _, ck := range c.creds.Cookies {
			req.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
		}
		if req.Method != http.MethodGet {
			req.Header.Set("X-CSRF-Token", c.creds.CSRF)
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
		}
	}
}

func checkStatus(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		resp.Body.Close()
		return ErrUnauthorized
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		loc := resp.Header.Get("Location")
		resp.Body.Close()
		if strings.Contains(loc, "/login") {
			return ErrUnauthorized
		}
		return &APIError{Status: resp.StatusCode, Message: "unexpected redirect to " + loc}
	case resp.StatusCode >= 400:
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return apiError(resp.StatusCode, b)
	}
	return nil
}

func apiError(status int, body []byte) error {
	var e struct {
		Errors  json.RawMessage `json:"errors"`
		Message string          `json:"message"`
	}
	msg := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &e) == nil {
		var list []struct{ Message string }
		switch {
		case e.Message != "":
			msg = e.Message
		case json.Unmarshal(e.Errors, &list) == nil && len(list) > 0:
			msg = list[0].Message
		case len(e.Errors) > 0:
			msg = string(e.Errors)
		}
	}
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	return &APIError{Status: status, Message: msg}
}

// throttle backs off when Canvas's leaky-bucket quota (X-Rate-Limit-Remaining, max 700) runs low.
func throttle(resp *http.Response) {
	rem, err := strconv.ParseFloat(resp.Header.Get("X-Rate-Limit-Remaining"), 64)
	if err == nil && rem < 100 {
		time.Sleep(time.Duration((100-rem)*20) * time.Millisecond)
	}
}

func (c *Client) Get(ctx context.Context, path string, q url.Values, out any) error {
	resp, err := c.do(ctx, http.MethodGet, c.url(path, q), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	resp, err := c.do(ctx, http.MethodPost, c.url(path, nil), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decode(resp, out)
}

func (c *Client) Delete(ctx context.Context, path string) error {
	resp, err := c.do(ctx, http.MethodDelete, c.url(path, nil), nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// Download streams rawURL (typically a file's signed `url`, which redirects to
// S3/inst-fs) into w. Redirects are followed; net/http drops the Authorization
// and Cookie headers when a redirect leaves the Canvas host.
func (c *Client) Download(ctx context.Context, rawURL string, w io.Writer) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "canvas-cli")
	c.authorize(req)
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return 0, ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return 0, apiError(resp.StatusCode, b)
	}
	return io.Copy(w, resp.Body)
}

func decode(resp *http.Response, out any) error {
	if out == nil {
		return nil
	}
	if raw, ok := out.(*json.RawMessage); ok {
		b, err := io.ReadAll(resp.Body)
		*raw = b
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// GetAll fetches every page of a list endpoint by following Link: rel="next".
func GetAll[T any](ctx context.Context, c *Client, path string, q url.Values) ([]T, error) {
	if q == nil {
		q = url.Values{}
	}
	if q.Get("per_page") == "" {
		q.Set("per_page", "100")
	}
	var all []T
	next := c.url(path, q)
	for next != "" {
		resp, err := c.do(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		var page []T
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		next = ""
		if m := nextLink.FindStringSubmatch(resp.Header.Get("Link")); m != nil {
			next = m[1]
		}
	}
	return all, nil
}
