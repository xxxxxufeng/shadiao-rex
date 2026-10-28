// Package history stores watch progress per series and decides which series
// are done (completed or caught up) and should leave recommendations.
package history

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

const debounce = time.Minute

type Entry struct {
	SeriesID      string    `json:"seriesId"`
	Title         string    `json:"title"`
	Cover         string    `json:"cover"`
	Total         int       `json:"total"`
	Finished      bool      `json:"finished"`
	LastEpisode   int       `json:"lastEpisode"`
	LastPlayedAt  time.Time `json:"lastPlayedAt"`
	Completed     bool      `json:"completed"`
	CaughtUp      bool      `json:"caughtUp"`
	CaughtUpTotal int       `json:"caughtUpTotal"`

	lastWrite time.Time
}

// HasUpdate reports new episodes since the series was caught up.
func (e Entry) HasUpdate(currentTotal int) bool {
	return e.CaughtUp && !e.Completed && currentTotal > e.CaughtUpTotal
}

type Store struct {
	mu      sync.Mutex
	file    string
	entries map[string]*Entry
	writes  int
}

func Open(file string) (*Store, error) {
	s := &Store{file: file, entries: map[string]*Entry{}}
	raw, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*Entry
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	for _, e := range list {
		s.entries[e.SeriesID] = e
	}
	return s, nil
}

func (s *Store) apply(e *Entry, show upstream.Show) {
	if show.Title != "" {
		e.Title = show.Title
	}
	if show.Cover != "" {
		e.Cover = show.Cover
	}
	if show.Episodes > 0 {
		e.Total = show.Episodes
	}
	e.Finished = show.Finished
}

// Record notes that episode of show started playing at now.
func (s *Store) Record(show upstream.Show, episode int, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[show.ID]
	if ok && e.LastEpisode == episode && now.Sub(e.lastWrite) < debounce {
		e.LastPlayedAt = now
		return nil
	}
	if !ok {
		e = &Entry{SeriesID: show.ID}
		s.entries[show.ID] = e
	}
	s.apply(e, show)
	e.LastEpisode, e.LastPlayedAt, e.lastWrite = episode, now, now
	if e.Total > 0 && episode >= e.Total {
		if e.Finished {
			e.Completed = true
		} else {
			e.CaughtUp, e.CaughtUpTotal = true, e.Total
		}
	}
	return s.save()
}

// Observe refreshes a recorded series' metadata (e.g. a grown episode count)
// from a fresh upstream view; unknown series are ignored.
func (s *Store) Observe(show upstream.Show) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[show.ID]
	if !ok {
		return
	}
	before := *e
	s.apply(e, show)
	if e.Total != before.Total || e.Finished != before.Finished || e.Title != before.Title || e.Cover != before.Cover {
		s.save()
	}
}

// Excluded reports series that should not be recommended again.
func (s *Store) Excluded(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return false
	}
	return e.Completed || (e.CaughtUp && !e.HasUpdate(e.Total))
}

func (s *Store) Get(id string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return Entry{}, false
	}
	return *e, true
}

func (s *Store) Recent() []Entry {
	s.mu.Lock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, *e)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].LastPlayedAt.After(out[j].LastPlayedAt) })
	return out
}

// save writes atomically; callers hold s.mu.
func (s *Store) save() error {
	list := make([]*Entry, 0, len(s.entries))
	for _, e := range s.entries {
		list = append(list, e)
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return err
	}
	tmp := s.file + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	s.writes++
	return os.Rename(tmp, s.file)
}
