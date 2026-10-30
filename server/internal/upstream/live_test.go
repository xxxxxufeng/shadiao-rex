//go:build live

package upstream

import (
	"context"
	"testing"
)

func TestLiveSearchAndResolve(t *testing.T) {
	h, err := NewSDK(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ids, _, err := h.SearchComic(ctx, "沙雕", "")
	if err != nil || len(ids) == 0 {
		t.Fatalf("search: %v %v", ids, err)
	}
	shows, err := h.Shows(ctx, ids)
	if err != nil || len(shows) != len(ids) || shows[0].Title == "" {
		t.Fatalf("shows: %d %v", len(shows), err)
	}
	eps, err := h.Episodes(ctx, ids[0])
	if err != nil || len(eps) == 0 || eps[0].No != 1 {
		t.Fatalf("episodes: %v", err)
	}
	p, err := h.Resolve(ctx, eps[0].ID, "720p")
	if err != nil || p.URL == "" || len(p.Key) != 16 || p.Size == 0 {
		t.Fatalf("resolve: %+v %v", p, err)
	}
	rel, err := h.Related(ctx, ids[0])
	if err != nil {
		t.Fatalf("related: %v", err)
	}
	t.Logf("related %d, first show %+v", len(rel), shows[0])
}

func TestLiveResolveRepeatedly(t *testing.T) {
	h, err := NewSDK(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	eps, err := h.Episodes(ctx, "7699701691242777624")
	if err != nil || len(eps) < 4 {
		t.Fatalf("episodes: %v", err)
	}
	// a fresh anonymous device serves one playback; Resolve must keep working
	for _, e := range eps[:4] {
		if p, err := h.Resolve(ctx, e.ID, "1080p"); err != nil || p.URL == "" {
			t.Fatalf("ep%d: %v", e.No, err)
		}
	}
}
