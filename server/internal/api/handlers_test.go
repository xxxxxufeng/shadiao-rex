package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/fake"
)

func TestListHotFiltersAndSorts(t *testing.T) {
	e := newEnv(t)
	got := decode[[]ShowDTO](t, e.get(t, "/api/list?kind=hot&page=1"))
	if len(got) != 2 || got[0].ID != "1001" || got[1].ID != "1002" {
		t.Fatalf("%+v", got)
	}
	if got[0].Cover == "" || len(got[0].Tags) == 0 || got[0].Created == 0 {
		t.Fatalf("fields %+v", got[0])
	}
}

func TestListExcludesCompleted(t *testing.T) {
	e := newEnv(t)
	e.hist.Record(e.f.ShowsBy["1001"], 3, time.Now())
	got := decode[[]ShowDTO](t, e.get(t, "/api/list?kind=hot"))
	if len(got) != 1 || got[0].ID != "1002" {
		t.Fatalf("%+v", got)
	}
}

func TestSeriesMarksLastEpisode(t *testing.T) {
	e := newEnv(t)
	e.hist.Record(e.f.ShowsBy["1001"], 2, time.Now())
	s := decode[SeriesDTO](t, e.get(t, "/api/series/1001?q=720p"))
	if s.LastEpisode != 2 || len(s.EpisodeList) != 3 {
		t.Fatalf("%+v", s)
	}
	u := s.EpisodeList[1].URL
	if u != "http://pub:8686/play/1001/2.mp4?q=720p&t=secret" {
		t.Fatalf("url %s", u)
	}
	// related keeps only 沙雕-pool-worthy, non-excluded shows
	if len(s.Related) != 1 || s.Related[0].ID != "1002" {
		t.Fatalf("related %+v", s.Related)
	}
}

func TestHistoryOrder(t *testing.T) {
	e := newEnv(t)
	now := time.Now()
	e.hist.Record(e.f.ShowsBy["1002"], 1, now.Add(-time.Hour))
	e.hist.Record(e.f.ShowsBy["1001"], 3, now)
	got := decode[[]ShowDTO](t, e.get(t, "/api/history"))
	if len(got) != 2 || got[0].ID != "1001" || !got[0].Completed || got[1].LastEpisode != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestHistoryShowsUpdate(t *testing.T) {
	e := newEnv(t)
	e.hist.Record(e.f.ShowsBy["1002"], 2, time.Now()) // caught up at 2
	s := e.f.ShowsBy["1002"]
	s.Episodes = 4
	e.f.ShowsBy["1002"] = s
	e.pool.Refresh(t.Context())
	got := decode[[]ShowDTO](t, e.get(t, "/api/history"))
	if !got[0].HasUpdate {
		t.Fatalf("%+v", got)
	}
	if l := decode[[]ShowDTO](t, e.get(t, "/api/list?kind=hot")); len(l) != 2 {
		t.Fatalf("updated show should be back in list: %+v", l)
	}
}

func TestSearchExcludes(t *testing.T) {
	e := newEnv(t)
	e.hist.Record(e.f.ShowsBy["1001"], 3, time.Now())
	got := decode[[]ShowDTO](t, e.get(t, "/api/search?q="+"%E6%B2%99%E9%9B%95"))
	// 1001 watched, 1003 has no 沙雕 style, 1004 is AI
	if len(got) != 1 || got[0].ID != "1002" {
		t.Fatalf("%+v", got)
	}
}

func TestUpstreamErrorShape(t *testing.T) {
	e := newEnv(t)
	e.f.Err = fake.ErrFake
	w := e.get(t, "/api/series/1009")
	if w.Code != 502 || !strings.Contains(w.Body.String(), `"source":"upstream"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	// list still served from pool
	if w := e.get(t, "/api/list?kind=hot"); w.Code != 200 {
		t.Fatalf("list %d", w.Code)
	}
}

func TestUnauthorizedNoUpstream(t *testing.T) {
	e := newEnv(t)
	for _, p := range []string{"/api/series/1001", "/play/1001/1.mp4", "/api/search?q=x"} {
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != 401 {
			t.Fatalf("%s %d", p, w.Code)
		}
	}
	if e.f.Calls != 0 {
		t.Fatalf("upstream calls %d", e.f.Calls)
	}
}

func TestSeriesRelatedFallsBackToPool(t *testing.T) {
	e := newEnv(t)
	e.f.Rel["1002"] = []string{"1003"} // only a non-沙雕 show upstream
	s := decode[SeriesDTO](t, e.get(t, "/api/series/1002"))
	if len(s.Related) != 1 || s.Related[0].ID != "1001" {
		t.Fatalf("related %+v", s.Related)
	}
}

func TestSearchExcludesAI(t *testing.T) {
	e := newEnv(t)
	for _, s := range decode[[]ShowDTO](t, e.get(t, "/api/search?q=%E6%B2%99%E9%9B%95")) {
		if s.ID == "1004" {
			t.Fatal("AI show in search results")
		}
	}
}

func TestListByCategoryAndTag(t *testing.T) {
	e := newEnv(t)
	s := e.f.ShowsBy["1002"]
	s.Tags = append(s.Tags, "规则怪谈")
	e.f.ShowsBy["1002"] = s
	e.pool.Refresh(t.Context())
	for _, q := range []string{"cat=horror", "tag=%E8%A7%84%E5%88%99%E6%80%AA%E8%B0%88"} {
		got := decode[[]ShowDTO](t, e.get(t, "/api/list?kind=hot&"+q))
		if len(got) != 1 || got[0].ID != "1002" {
			t.Fatalf("%s: %+v", q, got)
		}
	}
	if got := decode[[]ShowDTO](t, e.get(t, "/api/list?cat=nope")); len(got) != 0 {
		t.Fatalf("unknown cat %+v", got)
	}
}

func TestSearchIncludesPoolMatches(t *testing.T) {
	e := newEnv(t)
	s := e.f.ShowsBy["1002"]
	s.Tags = append(s.Tags, "修仙问道")
	e.f.ShowsBy["1002"] = s
	e.pool.Refresh(t.Context())
	// upstream search for 修仙 finds only an AI show
	e.f.Search["修仙"] = [][]string{{"1004"}}
	got := decode[[]ShowDTO](t, e.get(t, "/api/search?q=%E4%BF%AE%E4%BB%99"))
	if len(got) != 1 || got[0].ID != "1002" {
		t.Fatalf("%+v", got)
	}
	// title match too, and no duplicates when upstream also returns it
	e.f.Search["沙雕"] = [][]string{{"1001", "1002"}}
	got = decode[[]ShowDTO](t, e.get(t, "/api/search?q=%E6%B2%99%E9%9B%95"))
	if len(got) != 2 || got[0].ID != "1001" {
		t.Fatalf("%+v", got)
	}
	// pool matches only on page 1
	if got := decode[[]ShowDTO](t, e.get(t, "/api/search?q=%E4%BF%AE%E4%BB%99&page=2")); len(got) != 0 {
		t.Fatalf("page 2 %+v", got)
	}
}
