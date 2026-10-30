package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/cenc"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/config"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/history"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

// cdn serves the fixture with Range support; paths containing "expired" return 403.
func cdn(t *testing.T, enc []byte) (*httptest.Server, *int32) {
	var hits int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if strings.Contains(r.URL.Path, "expired") {
			http.Error(w, "expired", 403)
			return
		}
		http.ServeContent(w, r, "ep.mp4", time.Time{}, bytes.NewReader(enc))
	}))
	t.Cleanup(s.Close)
	return s, &hits
}

func playEnv(t *testing.T, urls ...string) (*env, []byte) {
	enc, key := fixture(t)
	e := newEnv(t)
	for _, u := range urls {
		e.f.Plays = append(e.f.Plays, &upstream.Play{URL: u, Key: key, Size: int64(len(enc)), ExpiresAt: time.Now().Add(6 * time.Hour)})
	}
	l, err := cenc.ParseAndRewrite(enc)
	if err != nil {
		t.Fatal(err)
	}
	full := append([]byte(nil), enc...)
	l.DecryptAt(key, 0, full)
	return e, full
}

func TestPlayFullDecrypts(t *testing.T) {
	enc, _ := fixture(t)
	c, _ := cdn(t, enc)
	e, full := playEnv(t, c.URL+"/ok")
	w := e.get(t, "/play/1001/1.mp4")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), full) {
		t.Fatalf("code %d len %d", w.Code, w.Body.Len())
	}
	if w.Header().Get("Accept-Ranges") != "bytes" || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("headers %v", w.Header())
	}
	p := filepath.Join(t.TempDir(), "out.mp4")
	os.WriteFile(p, w.Body.Bytes(), 0o644)
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", p, "-f", "null", "-").CombinedOutput(); err != nil || len(out) > 0 {
		t.Fatalf("ffmpeg decode: %v %s", err, out)
	}
}

func checkRange(t *testing.T, e *env, full []byte, from, to int) {
	t.Helper()
	w := e.get(t, "/play/1001/1.mp4", "Range", fmt.Sprintf("bytes=%d-%d", from, to))
	if w.Code != 206 {
		t.Fatalf("code %d", w.Code)
	}
	if want := fmt.Sprintf("bytes %d-%d/%d", from, to, len(full)); w.Header().Get("Content-Range") != want {
		t.Fatalf("content-range %q want %q", w.Header().Get("Content-Range"), want)
	}
	if !bytes.Equal(w.Body.Bytes(), full[from:to+1]) {
		t.Fatalf("range %d-%d bytes differ", from, to)
	}
}

func TestPlayRangeOverHeader(t *testing.T) {
	enc, _ := fixture(t)
	c, _ := cdn(t, enc)
	e, full := playEnv(t, c.URL+"/ok")
	checkRange(t, e, full, 0, 999)
	checkRange(t, e, full, 134000, 135999)
}

func TestPlayRangeMiddle(t *testing.T) {
	enc, _ := fixture(t)
	c, _ := cdn(t, enc)
	e, full := playEnv(t, c.URL+"/ok")
	checkRange(t, e, full, 3000000, 3100000)
	checkRange(t, e, full, len(full)-10, len(full)-1)
}

func TestPlayRetriesOnExpiredURL(t *testing.T) {
	enc, _ := fixture(t)
	c, _ := cdn(t, enc)
	e, full := playEnv(t, c.URL+"/expired", c.URL+"/ok")
	checkRange(t, e, full, 500000, 500999)
}

func TestPlayRecordsHistoryOnce(t *testing.T) {
	enc, _ := fixture(t)
	c, _ := cdn(t, enc)
	e, full := playEnv(t, c.URL+"/ok")
	for i := 0; i < 5; i++ {
		checkRange(t, e, full, i*1000, i*1000+99)
	}
	if h, ok := e.hist.Get("1001"); !ok || h.LastEpisode != 1 {
		t.Fatalf("%+v", h)
	}
	if e.f.Resolves != 1 {
		t.Fatalf("resolves %d (play info should be cached)", e.f.Resolves)
	}
}

func TestPlayRejectsUnknown(t *testing.T) {
	enc, _ := fixture(t)
	c, hits := cdn(t, enc)
	e, _ := playEnv(t, c.URL+"/ok")
	for _, p := range []string{"/play/abc/1.mp4", "/play/1001/99.mp4", "/play/1001/0.mp4", "/play/1001/x.mp4", "/play/1001/1.mp4?q=4k"} {
		if w := e.get(t, p); w.Code != 404 && w.Code != 400 {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
	if *hits != 0 {
		t.Fatalf("cdn hits %d", *hits)
	}
}

func TestPlayRecordUsesFreshEpisodeCount(t *testing.T) {
	enc, _ := fixture(t)
	c, _ := cdn(t, enc)
	e, _ := playEnv(t, c.URL+"/ok")
	// pool still says 3 episodes, but the live episode list has grown to 5
	e.f.Eps["1001"] = append(e.f.Eps["1001"], upstream.Episode{No: 4, ID: "e4"}, upstream.Episode{No: 5, ID: "e5"})
	if w := e.get(t, "/play/1001/3.mp4", "Range", "bytes=0-99"); w.Code != 206 {
		t.Fatalf("code %d", w.Code)
	}
	if h, _ := e.hist.Get("1001"); h.Completed || h.Total != 5 {
		t.Fatalf("stale total used: %+v", h)
	}
}

func TestPlayNotCacheable(t *testing.T) {
	enc, _ := fixture(t)
	c, _ := cdn(t, enc)
	e, _ := playEnv(t, c.URL+"/ok")
	w := e.get(t, "/play/1001/1.mp4", "Range", "bytes=0-99")
	// decrypted video must not be cached by CDNs/proxies (and plays must reach us to be recorded)
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control %q", got)
	}
}

func TestPlayPrefetchesNextEpisode(t *testing.T) {
	enc, key := fixture(t)
	c, hits := cdn(t, enc)
	e := newEnv(t)
	e.f.Plays = []*upstream.Play{{URL: c.URL + "/ok", Key: key, Size: int64(len(enc)), ExpiresAt: time.Now().Add(6 * time.Hour)}}
	dir := t.TempDir()
	e.h = NewMux(Deps{Cfg: &config.Config{Token: tok, DataDir: dir, CacheMB: 100}, Up: e.f, Pool: e.pool, Hist: e.hist, HTTP: http.DefaultClient})
	_, full := playEnv(t)
	e.f.Eps["1001"] = e.f.Eps["1001"][:2] // so playing 2 prefetches nothing further

	if w := e.get(t, "/play/1001/1.mp4", "Range", "bytes=0-99"); w.Code != 206 {
		t.Fatalf("code %d", w.Code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if m, _ := filepath.Glob(filepath.Join(dir, "cache", "*.mp4")); len(m) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("episode 2 never cached")
		}
		time.Sleep(20 * time.Millisecond)
	}
	before := atomic.LoadInt32(hits)
	w := e.get(t, "/play/1001/2.mp4")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), full) {
		t.Fatalf("cached full: code %d len %d", w.Code, w.Body.Len())
	}
	w = e.get(t, "/play/1001/2.mp4", "Range", "bytes=3000000-3000999")
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), full[3000000:3001000]) {
		t.Fatalf("cached range: code %d", w.Code)
	}
	if after := atomic.LoadInt32(hits); after != before {
		t.Fatalf("cached episode hit the cdn %d times", after-before)
	}
	if h, _ := e.hist.Get("1001"); h.LastEpisode != 2 {
		t.Fatalf("history %+v", h)
	}
}

func TestExtraTokenHasOwnHistory(t *testing.T) {
	enc, _ := fixture(t)
	c, _ := cdn(t, enc)
	e, _ := playEnv(t, c.URL+"/ok")
	other, _ := history.Open(filepath.Join(t.TempDir(), "history-friend.json"))
	e.h = NewMux(Deps{Cfg: &config.Config{Token: tok, PublicBase: "http://pub:8686"}, Up: e.f, Pool: e.pool, Hist: e.hist,
		Extra: map[string]*history.Store{"friend": other}, HTTP: http.DefaultClient})
	get := func(path string, hdr ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		for i := 0; i+1 < len(hdr); i += 2 {
			r.Header.Set(hdr[i], hdr[i+1])
		}
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, r)
		return w
	}
	if w := get("/play/1001/2.mp4?t=friend", "Range", "bytes=0-99"); w.Code != 206 {
		t.Fatalf("friend play %d", w.Code)
	}
	if h, ok := other.Get("1001"); !ok || h.LastEpisode != 2 {
		t.Fatalf("friend history %+v", h)
	}
	if _, ok := e.hist.Get("1001"); ok {
		t.Fatal("friend's play landed in the main history")
	}
	s := decode[SeriesDTO](t, get("/api/series/1001?t=friend"))
	if s.LastEpisode != 2 || !strings.HasSuffix(s.EpisodeList[0].URL, "&t=friend") {
		t.Fatalf("friend series: last %d url %s", s.LastEpisode, s.EpisodeList[0].URL)
	}
	if s := decode[SeriesDTO](t, e.get(t, "/api/series/1001")); s.LastEpisode != 0 {
		t.Fatalf("main sees friend's progress: %d", s.LastEpisode)
	}
	if w := get("/api/list?t=nope"); w.Code != 401 {
		t.Fatalf("unknown token %d", w.Code)
	}
}
