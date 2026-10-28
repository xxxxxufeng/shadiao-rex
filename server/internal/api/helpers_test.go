package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/config"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/fake"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/history"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/pool"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

const tok = "secret"

type env struct {
	f    *fake.Hongguo
	h    http.Handler
	hist *history.Store
	pool *pool.Pool
}

func show(id, title string, eps int, finished bool, heat int64, tags ...string) upstream.Show {
	return upstream.Show{ID: id, Title: title, Episodes: eps, Finished: finished, Heat: heat, Cover: "https://img/" + id + ".heic", Created: time.Unix(1700000000+heat, 0), Tags: tags}
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	f := &fake.Hongguo{
		Search: map[string][][]string{"沙雕": {{"1001", "1002", "1003", "1004"}}},
		ShowsBy: map[string]upstream.Show{
			"1001": show("1001", "沙雕甲", 3, true, 30, "搞笑", "沙雕"),
			"1002": show("1002", "乙", 2, false, 20, "表情包"),
			"1003": show("1003", "修仙丙", 5, true, 10, "玄幻"),
			"1004": show("1004", "AI丁", 5, true, 50, "轻松搞笑", "3D"),
		},
		Eps: map[string][]upstream.Episode{
			"1001": {{No: 1, ID: "e1"}, {No: 2, ID: "e2"}, {No: 3, ID: "e3"}},
			"1002": {{No: 1, ID: "f1"}, {No: 2, ID: "f2"}},
		},
		Rel: map[string][]string{"1001": {"1002", "1003"}},
	}
	p := pool.New(f, []string{"沙雕"}, 1, filepath.Join(dir, "pool.json"))
	if err := p.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	hist, _ := history.Open(filepath.Join(dir, "history.json"))
	cfg := &config.Config{Token: tok, PublicBase: "http://pub:8686"}
	h := NewMux(Deps{Cfg: cfg, Up: f, Pool: p, Hist: hist, HTTP: http.DefaultClient})
	f.Calls = 0
	return &env{f, h, hist, p}
}

func (e *env) get(t *testing.T, path string, hdr ...string) *httptest.ResponseRecorder {
	t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	r := httptest.NewRequest("GET", path+sep+"t="+tok, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %d %s: %v", w.Code, w.Body.String(), err)
	}
	return v
}

// fixture returns encrypted bytes and key, or skips.
func fixture(t *testing.T) ([]byte, []byte) {
	enc, err := os.ReadFile("../../testdata/ep.mp4")
	if err != nil {
		t.Skip("fixture missing")
	}
	k, _ := os.ReadFile("../../testdata/ep.key")
	key, _ := hex.DecodeString(strings.TrimSpace(string(k)))
	return enc, key
}
