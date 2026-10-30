package api

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xxxxxufeng/shadiao-rex/server/internal/cenc"
	"github.com/xxxxxufeng/shadiao-rex/server/internal/upstream"
)

// diskCache holds fully decrypted episodes so the next episode starts from
// local disk instead of a CDN round trip plus on-the-fly decryption.
type diskCache struct {
	dir string
	max int64

	mu       sync.Mutex
	inflight map[string]bool
	sem      chan struct{}
}

func newDiskCache(dir string, max int64) *diskCache {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("cache: %v (disabled)", err)
		return nil
	}
	parts, _ := filepath.Glob(filepath.Join(dir, "*.part"))
	for _, p := range parts {
		os.Remove(p) // left by a download cut short by a restart
	}
	return &diskCache{dir: dir, max: max, inflight: map[string]bool{}, sem: make(chan struct{}, 2)}
}

func (c *diskCache) path(k string) string {
	return filepath.Join(c.dir, fmt.Sprintf("%x.mp4", sha1.Sum([]byte(k))))
}

// open returns the cached file for k, bumping its mtime for LRU eviction.
func (c *diskCache) open(k string) *os.File {
	p := c.path(k)
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	now := time.Now()
	os.Chtimes(p, now, now)
	return f
}

// claim marks k as being fetched; false if it is cached or already in flight.
func (c *diskCache) claim(k string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight[k] {
		return false
	}
	if _, err := os.Stat(c.path(k)); err == nil {
		return false
	}
	c.inflight[k] = true
	return true
}

func (c *diskCache) release(k string) {
	c.mu.Lock()
	delete(c.inflight, k)
	c.mu.Unlock()
}

// evict deletes least recently used files until the cache fits in max.
func (c *diskCache) evict() {
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		mod  time.Time
	}
	var files []file
	var total int64
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".mp4") {
			continue
		}
		if info, err := e.Info(); err == nil {
			files = append(files, file{filepath.Join(c.dir, e.Name()), info.Size(), info.ModTime()})
			total += info.Size()
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	for _, f := range files {
		if total <= c.max {
			return
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}

// serveCached serves k from disk; false when it is not cached.
func (s *server) serveCached(w http.ResponseWriter, r *http.Request, k string) bool {
	if s.cache == nil {
		return false
	}
	f := s.cache.open(k)
	if f == nil {
		return false
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", "video/mp4")
	h.Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, "", time.Time{}, f)
	return true
}

// prefetchNext caches the episode after no in the background.
func (s *server) prefetchNext(eps []upstream.Episode, no int, q string) {
	if s.cache == nil {
		return
	}
	for _, ep := range eps {
		if ep.No != no+1 {
			continue
		}
		k := ep.ID + "|" + q
		if !s.cache.claim(k) {
			return
		}
		go func() {
			defer s.cache.release(k)
			s.cache.sem <- struct{}{}
			defer func() { <-s.cache.sem }()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			start := time.Now()
			var err error
			for attempt := 0; attempt < 2; attempt++ {
				if err = s.download(ctx, k, ep.ID, q, attempt > 0); !errors.Is(err, errExpired) {
					break
				}
			}
			if err != nil {
				log.Printf("prefetch %s: %v", k, err)
				return
			}
			log.Printf("prefetch %s: cached in %s", k, time.Since(start).Round(time.Millisecond))
			s.cache.evict()
		}()
		return
	}
}

// download writes the whole decrypted episode to the cache via a temp file.
func (s *server) download(ctx context.Context, k, episodeID, q string, fresh bool) error {
	p, err := s.resolve(ctx, episodeID, q, fresh)
	if err != nil {
		return err
	}
	if p.Size <= 0 {
		return fmt.Errorf("upstream: unknown file size")
	}
	var l *cenc.Layout
	if p.Key != nil {
		if l, err = s.layout(ctx, k, p); err != nil {
			return err
		}
	}
	resp, err := s.fetch(ctx, p.URL, 0, p.Size-1)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	tmp, err := os.CreateTemp(s.cache.dir, "dl-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	buf := make([]byte, chunk)
	var pos int64
	body := io.LimitReader(resp.Body, p.Size)
	for pos < p.Size {
		n, rerr := io.ReadFull(body, buf)
		if n > 0 {
			if l != nil {
				l.DecryptAt(p.Key, pos, buf[:n])
			}
			if _, err := tmp.Write(buf[:n]); err != nil {
				return err
			}
			pos += int64(n)
		}
		if rerr != nil {
			break
		}
	}
	if pos != p.Size {
		return fmt.Errorf("cdn body ended at %d of %d", pos, p.Size)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.cache.path(k))
}
