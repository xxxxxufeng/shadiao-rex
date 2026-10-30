// Package api serves hgsd's JSON API and decrypted video to the rex module.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/config"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/history"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/pool"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

const pageSize = 20

type Deps struct {
	Cfg  *config.Config
	Up   upstream.Hongguo
	Pool *pool.Pool
	Hist *history.Store // history of Cfg.Token
	// Extra maps each of Cfg.ExtraTokens' tokens to that user's own history.
	Extra map[string]*history.Store
	HTTP  *http.Client
}

type ShowDTO struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Cover       string   `json:"cover"`
	Episodes    int      `json:"episodes"`
	Finished    bool     `json:"finished"`
	Heat        int64    `json:"heat"`
	Created     int64    `json:"created"`
	Tags        []string `json:"tags"`
	LastEpisode int      `json:"lastEpisode,omitempty"`
	Completed   bool     `json:"completed,omitempty"`
	CaughtUp    bool     `json:"caughtUp,omitempty"`
	HasUpdate   bool     `json:"hasUpdate,omitempty"`
}

type EpisodeDTO struct {
	No       int    `json:"no"`
	Title    string `json:"title"`
	Duration int    `json:"duration"`
	URL      string `json:"url"`
}

type SeriesDTO struct {
	ShowDTO
	EpisodeList []EpisodeDTO `json:"episodeList"`
	Related     []ShowDTO    `json:"related"`
}

var seriesIDRe = regexp.MustCompile(`^\d{4,25}$`)

type userKey struct{}

// user is the caller's token and the history it reads and writes.
type user struct {
	token string
	hist  *history.Store
}

// RequireToken rejects requests lacking a known token (query t= or X-Token
// header) before next runs, so unauthorized callers never reach upstream.
// The matched token's history is passed on in the request context.
func RequireToken(users map[string]*history.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("t")
		if got == "" {
			got = r.Header.Get("X-Token")
		}
		var u *user
		for tok, h := range users {
			if tok != "" && subtle.ConstantTimeCompare([]byte(got), []byte(tok)) == 1 {
				u = &user{tok, h}
			}
		}
		if u == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	})
}

func userOf(r *http.Request) *user { return r.Context().Value(userKey{}).(*user) }

type server struct {
	Deps
	now func() time.Time

	mu       sync.Mutex
	shows    map[string]cached[upstream.Show]
	episodes map[string]cached[[]upstream.Episode]
	cursors  map[string]string // search q + "\x00" + page -> upstream cursor
	plays    map[string]*upstream.Play
	layouts  *layoutCache
	cache    *diskCache // nil when disabled
}

type cached[T any] struct {
	v     T
	until time.Time
}

func NewMux(d Deps) *http.ServeMux {
	s := &server{Deps: d, now: time.Now,
		shows: map[string]cached[upstream.Show]{}, episodes: map[string]cached[[]upstream.Episode]{},
		cursors: map[string]string{}, plays: map[string]*upstream.Play{}, layouts: newLayoutCache(32)}
	if d.Cfg.DataDir != "" && d.Cfg.CacheMB > 0 {
		s.cache = newDiskCache(filepath.Join(d.Cfg.DataDir, "cache"), int64(d.Cfg.CacheMB)<<20)
	}
	if s.HTTP == nil {
		s.HTTP = http.DefaultClient
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/list", s.list)
	protected.HandleFunc("GET /api/search", s.search)
	protected.HandleFunc("GET /api/series/{id}", s.series)
	protected.HandleFunc("GET /api/history", s.history)
	protected.HandleFunc("GET /play/{id}/{file}", s.play)
	users := map[string]*history.Store{d.Cfg.Token: d.Hist}
	for tok, h := range d.Extra {
		users[tok] = h
	}
	mux.Handle("/api/", RequireToken(users, protected))
	mux.Handle("/play/", RequireToken(users, protected))
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	code, source := http.StatusInternalServerError, "server"
	if strings.HasPrefix(err.Error(), "upstream:") {
		code, source = http.StatusBadGateway, "upstream"
	}
	log.Printf("api error: %v", err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "source": source})
}

func pageParam(r *http.Request) int {
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 {
		return 1
	}
	return p
}

func (s *server) dto(h *history.Store, sh upstream.Show) ShowDTO {
	d := ShowDTO{ID: sh.ID, Title: sh.Title, Description: sh.Description, Cover: sh.Cover,
		Episodes: sh.Episodes, Finished: sh.Finished, Heat: sh.Heat, Created: sh.Created.Unix(), Tags: sh.Tags}
	if d.Tags == nil {
		d.Tags = []string{}
	}
	if e, ok := h.Get(sh.ID); ok {
		d.LastEpisode, d.Completed, d.CaughtUp = e.LastEpisode, e.Completed, e.CaughtUp
		d.HasUpdate = e.HasUpdate(sh.Episodes)
	}
	return d
}

// observeHistory lets the history store see fresh episode counts from the pool,
// so caught-up series with new episodes come back.
func (s *server) observeHistory(h *history.Store) {
	for _, e := range h.Recent() {
		if sh, ok := s.Pool.Get(e.SeriesID); ok {
			h.Observe(sh)
		}
	}
}

func (s *server) list(w http.ResponseWriter, r *http.Request) {
	h := userOf(r).hist
	kind := r.URL.Query().Get("kind")
	if kind != "new" {
		kind = "hot"
	}
	s.observeHistory(h)
	out := []ShowDTO{}
	match := pool.Match(r.URL.Query().Get("cat"), r.URL.Query().Get("tag"))
	for _, sh := range s.Pool.ListMatch(kind, match, h.Excluded, pageParam(r), pageSize) {
		out = append(out, s.dto(h, sh))
	}
	writeJSON(w, out)
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	h := userOf(r).hist
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, []ShowDTO{})
		return
	}
	page := pageParam(r)
	out := []ShowDTO{}
	seen := map[string]bool{}
	if page == 1 {
		// the pool is searched locally too: upstream results for theme words are mostly 3D
		inPool := func(sh upstream.Show) bool {
			if strings.Contains(sh.Title, q) {
				return true
			}
			for _, t := range sh.Tags {
				if strings.Contains(t, q) {
					return true
				}
			}
			return false
		}
		for _, sh := range s.Pool.ListMatch("hot", inPool, h.Excluded, 1, pageSize) {
			seen[sh.ID] = true
			out = append(out, s.dto(h, sh))
		}
	}
	ids, err := s.searchPage(r.Context(), q, page)
	if err != nil {
		if len(out) > 0 {
			writeJSON(w, out)
			return
		}
		fail(w, err)
		return
	}
	shows, err := s.showsByID(r.Context(), ids)
	if err != nil {
		fail(w, err)
		return
	}
	for _, sh := range shows {
		if !seen[sh.ID] && !h.Excluded(sh.ID) && pool.Keep(sh) {
			seen[sh.ID] = true
			out = append(out, s.dto(h, sh))
		}
	}
	writeJSON(w, out)
}

// searchPage maps 1-based pages onto the upstream cursor chain, remembering cursors.
func (s *server) searchPage(ctx context.Context, q string, page int) ([]string, error) {
	key := func(p int) string { return q + "\x00" + strconv.Itoa(p) }
	start, cursor := 1, ""
	s.mu.Lock()
	if c, ok := s.cursors[key(page)]; ok {
		start, cursor = page, c
	}
	s.mu.Unlock()
	for p := start; ; p++ {
		ids, next, err := s.Up.SearchComic(ctx, q, cursor)
		if err != nil {
			return nil, err
		}
		if next != "" {
			s.mu.Lock()
			s.cursors[key(p+1)] = next
			s.mu.Unlock()
		}
		if p == page {
			return ids, nil
		}
		if next == "" {
			return nil, nil
		}
		cursor = next
	}
}

// showsByID returns shows in id order, from the pool, a 30-minute cache, or upstream.
func (s *server) showsByID(ctx context.Context, ids []string) ([]upstream.Show, error) {
	found := map[string]upstream.Show{}
	var missing []string
	s.mu.Lock()
	for _, id := range ids {
		if sh, ok := s.Pool.Get(id); ok {
			found[id] = sh
		} else if c, ok := s.shows[id]; ok && s.now().Before(c.until) {
			found[id] = c.v
		} else {
			missing = append(missing, id)
		}
	}
	s.mu.Unlock()
	if len(missing) > 0 {
		got, err := s.Up.Shows(ctx, missing)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		for _, sh := range got {
			found[sh.ID] = sh
			s.shows[sh.ID] = cached[upstream.Show]{sh, s.now().Add(30 * time.Minute)}
		}
		s.mu.Unlock()
	}
	out := make([]upstream.Show, 0, len(ids))
	for _, id := range ids {
		if sh, ok := found[id]; ok {
			out = append(out, sh)
		}
	}
	return out, nil
}

func (s *server) show(ctx context.Context, id string) (upstream.Show, error) {
	got, err := s.showsByID(ctx, []string{id})
	if err != nil {
		return upstream.Show{}, err
	}
	if len(got) == 0 {
		return upstream.Show{}, errNotFound
	}
	return got[0], nil
}

var errNotFound = errors.New("not found")

func (s *server) episodeList(ctx context.Context, id string) ([]upstream.Episode, error) {
	s.mu.Lock()
	c, ok := s.episodes[id]
	s.mu.Unlock()
	if ok && s.now().Before(c.until) {
		return c.v, nil
	}
	eps, err := s.Up.Episodes(ctx, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.episodes[id] = cached[[]upstream.Episode]{eps, s.now().Add(10 * time.Minute)}
	s.mu.Unlock()
	return eps, nil
}

func (s *server) publicBase(r *http.Request) string {
	if s.Cfg.PublicBase != "" {
		return strings.TrimRight(s.Cfg.PublicBase, "/")
	}
	return "http://" + r.Host
}

func (s *server) series(w http.ResponseWriter, r *http.Request) {
	u := userOf(r)
	h := u.hist
	id := r.PathValue("id")
	if !seriesIDRe.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	sh, err := s.show(r.Context(), id)
	if errors.Is(err, errNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	h.Observe(sh)
	eps, err := s.episodeList(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	q := quality(r.URL.Query().Get("q"))
	if q == "" {
		q = "1080p"
	}
	out := SeriesDTO{ShowDTO: s.dto(h, sh), EpisodeList: []EpisodeDTO{}, Related: []ShowDTO{}}
	base := s.publicBase(r)
	for _, e := range eps {
		u := fmt.Sprintf("%s/play/%s/%d.mp4?q=%s&t=%s", base, id, e.No, q, url.QueryEscape(u.token))
		out.EpisodeList = append(out.EpisodeList, EpisodeDTO{No: e.No, Title: e.Title, Duration: e.DurationSec, URL: u})
	}
	if relIDs, err := s.Up.Related(r.Context(), id); err == nil {
		if rel, err := s.showsByID(r.Context(), relIDs); err == nil {
			for _, rs := range rel {
				if rs.ID != id && pool.Keep(rs) && !h.Excluded(rs.ID) {
					out.Related = append(out.Related, s.dto(h, rs))
				}
			}
		}
	}
	if len(out.Related) == 0 {
		// anonymous devices get a generic "更多好剧" list; fall back to the pool
		for _, rs := range s.Pool.List("hot", h.Excluded, 1, 11) {
			if rs.ID != id && len(out.Related) < 10 {
				out.Related = append(out.Related, s.dto(h, rs))
			}
		}
	}
	writeJSON(w, out)
}

func (s *server) history(w http.ResponseWriter, r *http.Request) {
	h := userOf(r).hist
	s.observeHistory(h)
	out := []ShowDTO{}
	for _, e := range h.Recent() {
		sh, ok := s.Pool.Get(e.SeriesID)
		if !ok {
			sh = upstream.Show{ID: e.SeriesID, Title: e.Title, Cover: e.Cover, Episodes: e.Total, Finished: e.Finished}
		}
		out = append(out, s.dto(h, sh))
	}
	writeJSON(w, out)
}
