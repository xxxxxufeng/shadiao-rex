package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/cenc"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

const (
	headProbe = 256 << 10
	chunk     = 64 << 10
)

var errExpired = errors.New("cdn url expired")

func quality(q string) string {
	switch q {
	case "1080p", "720p", "540p", "480p", "360p":
		return q
	}
	return ""
}

type layoutCache struct {
	mu    sync.Mutex
	cap   int
	order []string
	m     map[string]*cenc.Layout
}

func newLayoutCache(n int) *layoutCache { return &layoutCache{cap: n, m: map[string]*cenc.Layout{}} }

func (c *layoutCache) get(k string) *cenc.Layout {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[k]
}

func (c *layoutCache) put(k string, l *cenc.Layout) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[k]; !ok {
		c.order = append(c.order, k)
		if len(c.order) > c.cap {
			delete(c.m, c.order[0])
			c.order = c.order[1:]
		}
	}
	c.m[k] = l
}

// resolve returns cached play info until 10 minutes before it expires.
func (s *server) resolve(ctx context.Context, episodeID, q string, fresh bool) (*upstream.Play, error) {
	k := episodeID + "|" + q
	s.mu.Lock()
	p, ok := s.plays[k]
	s.mu.Unlock()
	if ok && !fresh && s.now().Before(p.ExpiresAt.Add(-10*time.Minute)) {
		return p, nil
	}
	p, err := s.Up.Resolve(ctx, episodeID, q)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.plays[k] = p
	s.mu.Unlock()
	return p, nil
}

// fetch GETs [from, to] of the CDN file; errExpired on 403/410.
func (s *server) fetch(ctx context.Context, u string, from, to int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", from, to))
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream: cdn: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusPartialContent, http.StatusOK:
		return resp, nil
	case http.StatusForbidden, http.StatusGone:
		resp.Body.Close()
		return nil, errExpired
	default:
		resp.Body.Close()
		return nil, fmt.Errorf("upstream: cdn status %d", resp.StatusCode)
	}
}

func (s *server) readRange(ctx context.Context, u string, from, to int64) ([]byte, error) {
	resp, err := s.fetch(ctx, u, from, to)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && from > 0 {
		return nil, fmt.Errorf("upstream: cdn ignored range")
	}
	return io.ReadAll(io.LimitReader(resp.Body, to-from+1))
}

func (s *server) layout(ctx context.Context, k string, p *upstream.Play) (*cenc.Layout, error) {
	if l := s.layouts.get(k); l != nil {
		return l, nil
	}
	head, err := s.readRange(ctx, p.URL, 0, headProbe-1)
	if err != nil {
		return nil, err
	}
	need, err := cenc.NeedHead(head)
	if err != nil {
		return nil, err
	}
	if need > int64(len(head)) {
		if head, err = s.readRange(ctx, p.URL, 0, need-1); err != nil {
			return nil, err
		}
	}
	l, err := cenc.ParseAndRewrite(head)
	if err != nil {
		return nil, err
	}
	s.layouts.put(k, l)
	return l, nil
}

// parseRange handles a single "bytes=" range; ok=false means no/unsupported header.
func parseRange(h string, size int64) (from, to int64, ok bool, valid bool) {
	if !strings.HasPrefix(h, "bytes=") || strings.Contains(h, ",") {
		return 0, size - 1, false, true
	}
	a, b, _ := strings.Cut(strings.TrimPrefix(h, "bytes="), "-")
	var err error
	switch {
	case a == "":
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, true, false
		}
		from, to = max(size-n, 0), size-1
	default:
		if from, err = strconv.ParseInt(a, 10, 64); err != nil {
			return 0, 0, true, false
		}
		to = size - 1
		if b != "" {
			if to, err = strconv.ParseInt(b, 10, 64); err != nil {
				return 0, 0, true, false
			}
			to = min(to, size-1)
		}
	}
	return from, to, true, from <= to && from < size
}

func (s *server) play(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	no, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("file"), ".mp4"))
	if !seriesIDRe.MatchString(id) || err != nil || no < 1 {
		http.NotFound(w, r)
		return
	}
	q := "1080p"
	if raw := r.URL.Query().Get("q"); raw != "" {
		if q = quality(raw); q == "" {
			http.Error(w, "bad quality", http.StatusBadRequest)
			return
		}
	}
	ctx := r.Context()
	eps, err := s.episodeList(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	var ep *upstream.Episode
	for i := range eps {
		if eps[i].No == no {
			ep = &eps[i]
		}
	}
	if ep == nil {
		http.NotFound(w, r)
		return
	}
	k := ep.ID + "|" + q
	if !s.serveCached(w, r, k) {
		for attempt := 0; attempt < 2; attempt++ {
			err = s.serveEpisode(w, r, k, ep.ID, q, attempt > 0)
			if !errors.Is(err, errExpired) {
				break
			}
			log.Printf("play %s: cdn url expired, re-resolving", k)
		}
		if err != nil {
			fail(w, err)
			return
		}
	}
	if r.Method == http.MethodGet {
		s.prefetchNext(eps, no, q)
	}
	if sh, err := s.show(ctx, id); err == nil {
		if n := len(eps); n > sh.Episodes {
			sh.Episodes = n // the live episode list is fresher than the pool
		}
		userOf(r).hist.Record(sh, no, s.now())
	}
}

// serveEpisode writes the (decrypted) range; it returns errExpired before
// writing anything so the caller can retry with a fresh URL.
func (s *server) serveEpisode(w http.ResponseWriter, r *http.Request, k, episodeID, q string, fresh bool) error {
	ctx := r.Context()
	p, err := s.resolve(ctx, episodeID, q, fresh)
	if err != nil {
		return err
	}
	var l *cenc.Layout
	if p.Key != nil {
		if l, err = s.layout(ctx, k, p); err != nil {
			return err
		}
	}
	size := p.Size
	if size <= 0 {
		return fmt.Errorf("upstream: unknown file size")
	}
	from, to, partial, valid := parseRange(r.Header.Get("Range"), size)
	if !valid {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
		http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
		return nil
	}
	resp, err := s.fetch(ctx, p.URL, from, to)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && from > 0 {
		return fmt.Errorf("upstream: cdn ignored range")
	}
	h := w.Header()
	h.Set("Content-Type", "video/mp4")
	h.Set("Cache-Control", "private, no-store")
	h.Set("Accept-Ranges", "bytes")
	h.Set("Content-Length", strconv.FormatInt(to-from+1, 10))
	if partial {
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodHead {
		return nil
	}
	buf := make([]byte, chunk)
	pos := from
	body := io.LimitReader(resp.Body, to-from+1)
	for pos <= to {
		n, rerr := io.ReadFull(body, buf)
		if n > 0 {
			if l != nil {
				l.DecryptAt(p.Key, pos, buf[:n])
			}
			if _, werr := w.Write(buf[:n]); werr != nil {
				return nil // client went away
			}
			pos += int64(n)
		}
		if rerr != nil {
			if pos <= to {
				log.Printf("play %s: cdn body ended at %d of %d: %v", k, pos, to, rerr)
			}
			return nil
		}
	}
	return nil
}
