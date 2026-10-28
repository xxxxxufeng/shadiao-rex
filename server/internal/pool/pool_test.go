package pool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/fake"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

func show(id, title string, heat int64, day int, tags ...string) upstream.Show {
	return upstream.Show{ID: id, Title: title, Heat: heat, Created: time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC), Tags: tags}
}

func TestKeep(t *testing.T) {
	cases := []struct {
		s    upstream.Show
		want bool
	}{
		{show("1", "坑爹悍将", 0, 1, "喜剧", "沙雕", "搞笑"), true},
		{show("2", "绝境黑夜", 0, 1, "玄幻", "古代", "表情包"), true},
		{show("3", "高冷千金的贴身高手", 0, 1, "爱情", "3D", "轻松搞笑"), false},
		{show("4", "沙雕女主荒岛爆笑求生记", 0, 1, "冒险", "轻松搞笑", "3D漫"), false},
		{show("5", "沙雕修仙系统", 0, 1, "玄幻", "搞笑"), false}, // no style tag: unknown, leave out
	}
	for _, c := range cases {
		if Keep(c.s) != c.want {
			t.Fatalf("%s: want %v", c.s.Title, c.want)
		}
	}
}

func TestIsAI(t *testing.T) {
	if !IsAI(show("1", "x", 0, 1, "3D")) || !IsAI(show("1", "x", 0, 1, "3D漫")) || IsAI(show("1", "x", 0, 1, "表情包")) {
		t.Fatal("IsAI")
	}
}

func newFake() *fake.Hongguo {
	return &fake.Hongguo{
		Search: map[string][][]string{
			"沙雕": {{"a", "b"}, {"c"}},
			"搞笑": {{"b", "d", "x"}},
		},
		ShowsBy: map[string]upstream.Show{
			"a": show("a", "沙雕A", 10, 5, "沙雕"),
			"b": show("b", "B", 30, 1, "搞笑", "表情包"),
			"c": show("c", "C", 20, 9, "沙雕"),
			"d": show("d", "D", 40, 3, "表情包"),
			"x": show("x", "修仙X", 99, 9, "玄幻", "3D漫"),
		},
	}
}

func ids(ss []upstream.Show) (out []string) {
	for _, s := range ss {
		out = append(out, s.ID)
	}
	return
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRefreshDedupAndSort(t *testing.T) {
	p := New(newFake(), []string{"沙雕", "搞笑"}, 3, filepath.Join(t.TempDir(), "pool.json"))
	if err := p.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := ids(p.List("hot", nil, 1, 20)); !eq(got, []string{"d", "b", "c", "a"}) {
		t.Fatalf("hot %v", got)
	}
	if got := ids(p.List("new", nil, 1, 20)); !eq(got, []string{"c", "a", "d", "b"}) {
		t.Fatalf("new %v", got)
	}
	if got := ids(p.List("hot", nil, 2, 3)); !eq(got, []string{"a"}) {
		t.Fatalf("page 2 %v", got)
	}
}

func TestExclude(t *testing.T) {
	p := New(newFake(), []string{"沙雕", "搞笑"}, 3, filepath.Join(t.TempDir(), "pool.json"))
	p.Refresh(context.Background())
	ex := func(id string) bool { return id == "d" || id == "c" }
	if got := ids(p.List("hot", ex, 1, 1)); !eq(got, []string{"b"}) {
		t.Fatalf("page1 %v", got)
	}
	if got := ids(p.List("hot", ex, 2, 1)); !eq(got, []string{"a"}) {
		t.Fatalf("page2 %v", got)
	}
}

func TestRefreshFailureKeepsOldPool(t *testing.T) {
	f := newFake()
	p := New(f, []string{"沙雕", "搞笑"}, 3, filepath.Join(t.TempDir(), "pool.json"))
	p.Refresh(context.Background())
	f.Err = fake.ErrFake
	if err := p.Refresh(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if got := ids(p.List("hot", nil, 1, 20)); len(got) != 4 {
		t.Fatalf("pool changed: %v", got)
	}
}

func TestPersistence(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pool.json")
	p := New(newFake(), []string{"沙雕", "搞笑"}, 3, file)
	p.Refresh(context.Background())
	q := New(newFake(), nil, 3, file)
	if err := q.Load(); err != nil {
		t.Fatal(err)
	}
	if !eq(ids(q.List("hot", nil, 1, 20)), ids(p.List("hot", nil, 1, 20))) {
		t.Fatal("persisted pool differs")
	}
	if s, ok := q.Get("b"); !ok || s.Title != "B" {
		t.Fatal("Get after load")
	}
}

func TestEmptyRefreshKeepsOldPool(t *testing.T) {
	f := newFake()
	p := New(f, []string{"沙雕", "搞笑"}, 3, filepath.Join(t.TempDir(), "pool.json"))
	p.Refresh(context.Background())
	f.ShowsBy = map[string]upstream.Show{} // ids found but no details (soft limit)
	if err := p.Refresh(context.Background()); err == nil {
		t.Fatal("expected error for empty refresh")
	}
	if p.Len() != 4 {
		t.Fatalf("pool wiped: %d", p.Len())
	}
	q := New(f, nil, 3, p.file)
	q.Load()
	if q.Len() != 4 {
		t.Fatalf("persisted pool wiped: %d", q.Len())
	}
}

func TestRunRetriesWhileEmpty(t *testing.T) {
	f := newFake()
	f.Err = fake.ErrFake
	p := New(f, []string{"沙雕", "搞笑"}, 3, filepath.Join(t.TempDir(), "pool.json"))
	p.retry = 20 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx, time.Hour)
	time.Sleep(50 * time.Millisecond)
	f.SetErr(nil)
	deadline := time.Now().Add(2 * time.Second)
	for p.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.Len() != 4 {
		t.Fatalf("pool not filled by retry: %d", p.Len())
	}
}

func TestCategories(t *testing.T) {
	horror := show("h", "规则", 1, 1, "沙雕", "规则怪谈")
	funny := show("f", "笑", 1, 1, "表情包", "轻松搞笑")
	if !Match("horror", "")(horror) || Match("horror", "")(funny) || !Match("funny", "")(funny) {
		t.Fatal("category match")
	}
	if !Match("", "规则怪谈")(horror) || Match("", "规则怪谈")(funny) {
		t.Fatal("tag match")
	}
	if Match("nope", "")(funny) {
		t.Fatal("unknown category should match nothing")
	}
	if !Match("", "")(funny) {
		t.Fatal("empty matches all")
	}
	ids := map[string]bool{}
	for _, c := range Categories {
		if c.ID == "" || c.Title == "" || len(c.Tags) == 0 || ids[c.ID] {
			t.Fatalf("bad category %+v", c)
		}
		ids[c.ID] = true
	}
}

func TestListWithMatch(t *testing.T) {
	p := New(newFake(), []string{"沙雕", "搞笑"}, 3, filepath.Join(t.TempDir(), "pool.json"))
	p.Refresh(context.Background())
	got := ids(p.ListMatch("hot", func(s upstream.Show) bool { return s.ID == "b" || s.ID == "a" }, nil, 1, 20))
	if !eq(got, []string{"b", "a"}) {
		t.Fatalf("%v", got)
	}
}

func TestCategoriesNotTooBroad(t *testing.T) {
	// a modern-day 规则怪谈 show with generic tags belongs to horror only
	s := show("g", "全球杀机", 1, 1, "沙雕", "奇幻脑洞", "现代", "大男主", "异能", "规则怪谈", "穿越")
	var in []string
	for _, c := range Categories {
		if Match(c.ID, "")(s) {
			in = append(in, c.ID)
		}
	}
	if !eq(in, []string{"horror"}) {
		t.Fatalf("categories %v", in)
	}
}
