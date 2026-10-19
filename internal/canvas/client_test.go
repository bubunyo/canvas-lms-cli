package canvas

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bubunyo/canvas-cli/internal/creds"
)

func TestGetAllFollowsLinkHeader(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/api/v1/courses?page=2&per_page=100>; rel="next", <%s/api/v1/courses?page=1>; rel="first"`, srv.URL, srv.URL))
			fmt.Fprint(w, `[{"id":1,"name":"A"}]`)
		case "2":
			fmt.Fprint(w, `[{"id":"2","name":"B"}]`)
		}
	}))
	defer srv.Close()

	cl := New(srv.URL, &creds.Credentials{Kind: creds.KindToken, Token: "tok"})
	got, err := GetAll[Course](context.Background(), cl, "/api/v1/courses", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("got %+v", got)
	}
}

func TestCookieAuthSendsCSRFOnWrites(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("_normandy_session"); err != nil || c.Value != "sess" {
			t.Errorf("missing session cookie")
		}
		csrf := r.Header.Get("X-CSRF-Token")
		if r.Method == http.MethodGet && csrf != "" {
			t.Errorf("GET should not send CSRF token")
		}
		if r.Method == http.MethodPost && csrf != "csrf==" {
			t.Errorf("POST X-CSRF-Token = %q", csrf)
		}
		fmt.Fprint(w, `{"id":7,"token":"new-token"}`)
	}))
	defer srv.Close()

	cl := New(srv.URL, &creds.Credentials{Kind: creds.KindCookie, Cookies: []creds.Cookie{{Name: "_normandy_session", Value: "sess"}}, CSRF: "csrf=="})
	var u User
	if err := cl.Get(context.Background(), "/api/v1/users/self", nil, &u); err != nil {
		t.Fatal(err)
	}
	var at AccessToken
	if err := cl.Post(context.Background(), "/api/v1/users/self/tokens", map[string]any{}, &at); err != nil {
		t.Fatal(err)
	}
	if at.ID != "7" || at.Token != "new-token" {
		t.Fatalf("got %+v", at)
	}
}

func TestExpiredSessionRedirectIsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	}))
	defer srv.Close()
	err := New(srv.URL, &creds.Credentials{Kind: creds.KindCookie}).Get(context.Background(), "/api/v1/users/self", nil, &User{})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v", err)
	}
}

func TestRateLimitRetries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "403 Forbidden (Rate Limit Exceeded)", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `{"id":1}`)
	}))
	defer srv.Close()
	if err := New(srv.URL, nil).Get(context.Background(), "/x", nil, &User{}); err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestAPIErrorMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"errors":[{"message":"user not authorized to perform that action"}]}`)
	}))
	defer srv.Close()
	err := New(srv.URL, nil).Post(context.Background(), "/x", nil, nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Message != "user not authorized to perform that action" {
		t.Fatalf("err = %v", err)
	}
}
