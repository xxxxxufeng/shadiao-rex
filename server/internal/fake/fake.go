// Package fake is an in-memory upstream.Hongguo for tests.
package fake

import (
	"context"
	"errors"
	"sync"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

type Hongguo struct {
	mu       sync.Mutex
	Search   map[string][][]string // keyword -> pages of ids
	ShowsBy  map[string]upstream.Show
	Eps      map[string][]upstream.Episode
	Rel      map[string][]string
	Plays    []*upstream.Play // returned in order by Resolve; last one repeats
	Err      error            // when set, every call fails with it
	Calls    int
	Resolves int
}

var ErrFake = errors.New("upstream: fake failure")

func (f *Hongguo) SetErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Err = err
}

func (f *Hongguo) hit() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	return f.Err
}

func (f *Hongguo) SearchComic(_ context.Context, q, cursor string) ([]string, string, error) {
	if err := f.hit(); err != nil {
		return nil, "", err
	}
	pages := f.Search[q]
	i := 0
	if cursor != "" {
		i = int(cursor[0] - '0')
	}
	if i >= len(pages) {
		return nil, "", nil
	}
	next := ""
	if i+1 < len(pages) {
		next = string(rune('0' + i + 1))
	}
	return pages[i], next, nil
}

func (f *Hongguo) Shows(_ context.Context, ids []string) ([]upstream.Show, error) {
	if err := f.hit(); err != nil {
		return nil, err
	}
	var out []upstream.Show
	for _, id := range ids {
		if s, ok := f.ShowsBy[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *Hongguo) Episodes(_ context.Context, id string) ([]upstream.Episode, error) {
	if err := f.hit(); err != nil {
		return nil, err
	}
	return f.Eps[id], nil
}

func (f *Hongguo) Related(_ context.Context, id string) ([]string, error) {
	if err := f.hit(); err != nil {
		return nil, err
	}
	return f.Rel[id], nil
}

func (f *Hongguo) Resolve(_ context.Context, _, _ string) (*upstream.Play, error) {
	if err := f.hit(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i := min(f.Resolves, len(f.Plays)-1)
	f.Resolves++
	p := *f.Plays[i]
	return &p, nil
}
