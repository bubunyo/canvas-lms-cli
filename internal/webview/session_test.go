package webview

import "testing"

func TestParseCookies(t *testing.T) {
	if s, _ := parseCookies("log_session_id\tx\n_csrf_token\tabc\n"); s != nil {
		t.Fatal("no session cookie: want nil")
	}
	s, names := parseCookies("_csrf_token\ta%2Bb%3D%3D\ncanvas_session\tsess\n")
	if s == nil || s.CSRF != "a+b==" || len(s.Cookies) != 2 || len(names) != 2 {
		t.Fatalf("got %+v %v", s, names)
	}
}
