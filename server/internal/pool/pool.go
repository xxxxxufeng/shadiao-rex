// Package pool keeps the "沙雕" pool: comics found by seed keywords, filtered
// by tag/title, persisted to disk and refreshed periodically.
package pool

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

type Pool struct {
	h        upstream.Hongguo
	keywords []string
	pages    int
	file     string
	retry    time.Duration // refresh interval while the pool is empty

	mu    sync.RWMutex
	shows map[string]upstream.Show
}

func New(h upstream.Hongguo, keywords []string, pagesPerKeyword int, file string) *Pool {
	return &Pool{h: h, keywords: keywords, pages: pagesPerKeyword, file: file, retry: 10 * time.Minute, shows: map[string]upstream.Show{}}
}

// Keep reports whether a show is drawn in the 沙雕 style. Style tags only come
// from a metadata device on a styled taxonomy: "沙雕"/"表情包" mark the meme
// style, "3D"/"3D漫" mark AI-rendered shows. No style tag means unknown: left out.
func Keep(s upstream.Show) bool {
	for _, t := range s.Tags {
		if t == "沙雕" || t == "表情包" {
			return true
		}
	}
	return false
}

// IsAI reports AI-rendered (3D) shows.
func IsAI(s upstream.Show) bool {
	for _, t := range s.Tags {
		if strings.HasPrefix(t, "3D") {
			return true
		}
	}
	return false
}

func (p *Pool) Load() error {
	raw, err := os.ReadFile(p.file)
	if err != nil {
		return err
	}
	var list []upstream.Show
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	p.replace(list)
	return nil
}

func (p *Pool) replace(list []upstream.Show) {
	m := make(map[string]upstream.Show, len(list))
	for _, s := range list {
		m[s.ID] = s
	}
	p.mu.Lock()
	p.shows = m
	p.mu.Unlock()
}

// Refresh rebuilds the pool; on any upstream failure the current pool is kept.
func (p *Pool) Refresh(ctx context.Context) error {
	var ids []string
	seen := map[string]bool{}
	for _, kw := range p.keywords {
		cursor := ""
		for page := 0; page < p.pages; page++ {
			got, next, err := p.h.SearchComic(ctx, kw, cursor)
			if err != nil {
				return err
			}
			for _, id := range got {
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
				}
			}
			if next == "" {
				break
			}
			cursor = next
		}
	}
	shows, err := p.h.Shows(ctx, ids)
	if err != nil {
		return err
	}
	var kept []upstream.Show
	for _, s := range shows {
		if Keep(s) {
			kept = append(kept, s)
		}
	}
	log.Printf("pool: %d ids from search, %d shows resolved, %d kept", len(ids), len(shows), len(kept))
	if len(kept) == 0 {
		// a soft-limited device returns ids without details; never swap in an empty pool
		return errors.New("upstream: refresh produced an empty pool")
	}
	p.replace(kept)
	raw, _ := json.Marshal(kept)
	tmp := p.file + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.file)
}

// List returns one page (1-based) of the pool sorted by kind ("hot": heat,
// "new": creation time), skipping ids for which exclude returns true.
func (p *Pool) List(kind string, exclude func(id string) bool, page, size int) []upstream.Show {
	return p.ListMatch(kind, nil, exclude, page, size)
}

// ListMatch is List restricted to shows for which match returns true (nil: all).
func (p *Pool) ListMatch(kind string, match func(upstream.Show) bool, exclude func(id string) bool, page, size int) []upstream.Show {
	p.mu.RLock()
	all := make([]upstream.Show, 0, len(p.shows))
	for _, s := range p.shows {
		if (exclude == nil || !exclude(s.ID)) && (match == nil || match(s)) {
			all = append(all, s)
		}
	}
	p.mu.RUnlock()
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if kind == "new" && !a.Created.Equal(b.Created) {
			return a.Created.After(b.Created)
		}
		if a.Heat != b.Heat {
			return a.Heat > b.Heat
		}
		return a.ID < b.ID
	})
	if page < 1 {
		page = 1
	}
	from := (page - 1) * size
	if from >= len(all) {
		return nil
	}
	return all[from:min(from+size, len(all))]
}

func (p *Pool) Get(id string) (upstream.Show, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	s, ok := p.shows[id]
	return s, ok
}

func (p *Pool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.shows)
}

// Run loads the persisted pool, refreshes immediately if empty, then every interval.
func (p *Pool) Run(ctx context.Context, every time.Duration) {
	if err := p.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("pool: load: %v", err)
	}
	refresh := func() {
		if err := p.Refresh(ctx); err != nil {
			log.Printf("pool: refresh failed, keeping %d shows: %v", p.Len(), err)
			return
		}
		log.Printf("pool: refreshed, %d shows", p.Len())
	}
	if p.Len() == 0 {
		refresh()
	}
	for {
		wait := every
		if p.Len() == 0 {
			wait = p.retry
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			refresh()
		}
	}
}
