package api

import (
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/config"
)

func TestRequireToken(t *testing.T) {
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ })
	h := RequireToken("secret", next)
	cases := []struct {
		url, header string
		want        int
	}{
		{"/api/list", "", 401},
		{"/api/list?t=wrong", "", 401},
		{"/api/list", "nope", 401},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", c.url, nil)
		if c.header != "" {
			r.Header.Set("X-Token", c.header)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatalf("%s: got %d", c.url, w.Code)
		}
	}
	if calls != 0 {
		t.Fatalf("next called %d times on unauthorized", calls)
	}
	for _, r := range []*http.Request{httptest.NewRequest("GET", "/api/list?t=secret", nil), func() *http.Request {
		r := httptest.NewRequest("GET", "/api/list", nil)
		r.Header.Set("X-Token", "secret")
		return r
	}()} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("authorized got %d", w.Code)
		}
	}
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
}

func TestHealthzNoToken(t *testing.T) {
	mux := NewMux(Deps{Cfg: &config.Config{Token: "secret"}})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 || w.Body.String() != "ok" {
		t.Fatalf("healthz %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/list", nil))
	if w.Code != 401 {
		t.Fatalf("api without token %d", w.Code)
	}
}

func TestAccessLogHidesToken(t *testing.T) {
	var buf strings.Builder
	h := AccessLog(log.New(&buf, "", 0), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(418) }))
	r := httptest.NewRequest("GET", "/api/list?kind=hot&t=secret", nil)
	r.RemoteAddr = "1.2.3.4:5555"
	r.Header.Set("Range", "bytes=0-1")
	h.ServeHTTP(httptest.NewRecorder(), r)
	line := buf.String()
	if strings.Contains(line, "secret") || !strings.Contains(line, "1.2.3.4") || !strings.Contains(line, "418") || !strings.Contains(line, "/api/list") || !strings.Contains(line, "bytes=0-1") {
		t.Fatalf("log line %q", line)
	}
}
