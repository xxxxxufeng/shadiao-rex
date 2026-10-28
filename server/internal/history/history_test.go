package history

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

var t0 = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func open(t *testing.T) (*Store, string) {
	f := filepath.Join(t.TempDir(), "history.json")
	s, err := Open(f)
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

func fin(id string, total int) upstream.Show {
	return upstream.Show{ID: id, Title: "T" + id, Episodes: total, Finished: true}
}
func upd(id string, total int) upstream.Show {
	return upstream.Show{ID: id, Title: "T" + id, Episodes: total}
}

func TestCompletedFinishedSeries(t *testing.T) {
	s, _ := open(t)
	s.Record(fin("a", 10), 9, t0)
	if s.Excluded("a") {
		t.Fatal("excluded before last episode")
	}
	s.Record(fin("a", 10), 10, t0.Add(time.Hour))
	e, _ := s.Get("a")
	if !e.Completed || !s.Excluded("a") {
		t.Fatalf("not completed: %+v", e)
	}
}

func TestCaughtUpUpdating(t *testing.T) {
	s, _ := open(t)
	s.Record(upd("b", 20), 20, t0)
	e, _ := s.Get("b")
	if !e.CaughtUp || e.Completed || !s.Excluded("b") {
		t.Fatalf("caught up: %+v", e)
	}
	s.Observe(upd("b", 25))
	e, _ = s.Get("b")
	if !e.HasUpdate(25) || s.Excluded("b") {
		t.Fatalf("after update: %+v excluded=%v", e, s.Excluded("b"))
	}
	s.Record(upd("b", 25), 25, t0.Add(time.Hour))
	if !s.Excluded("b") {
		t.Fatal("not re-caught-up")
	}
}

func TestRecordDoesNotRegress(t *testing.T) {
	s, _ := open(t)
	s.Record(fin("a", 10), 10, t0)
	s.Record(fin("a", 10), 1, t0.Add(time.Hour))
	e, _ := s.Get("a")
	if !e.Completed || e.LastEpisode != 1 {
		t.Fatalf("%+v", e)
	}
}

func TestRecentOrder(t *testing.T) {
	s, _ := open(t)
	s.Record(fin("a", 10), 1, t0)
	s.Record(fin("b", 10), 1, t0.Add(time.Minute))
	s.Record(fin("c", 10), 1, t0.Add(-time.Minute))
	r := s.Recent()
	if len(r) != 3 || r[0].SeriesID != "b" || r[2].SeriesID != "c" {
		t.Fatalf("%+v", r)
	}
}

func TestPersistAcrossOpen(t *testing.T) {
	s, f := open(t)
	s.Record(fin("a", 10), 10, t0)
	s2, err := Open(f)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := s2.Get("a"); !ok || !e.Completed || e.Title != "Ta" {
		t.Fatalf("%+v", e)
	}
}

func TestDebounce(t *testing.T) {
	s, _ := open(t)
	for i := 0; i < 10; i++ {
		s.Record(fin("a", 10), 3, t0.Add(time.Duration(i)*time.Second))
	}
	if s.writes != 1 {
		t.Fatalf("writes %d", s.writes)
	}
	s.Record(fin("a", 10), 3, t0.Add(2*time.Minute))
	if s.writes != 2 {
		t.Fatalf("writes after window %d", s.writes)
	}
}
