// Package upstream is the only place hgsd touches openhongguo; everything else
// depends on the Hongguo interface so it can be faked in tests.
package upstream

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/beilunyang/openhongguo/adapters"
	"github.com/beilunyang/openhongguo/sdk"
	"github.com/beilunyang/openhongguo/sdk/model"
)

type Show struct {
	ID, Title, Description, Cover string
	Episodes                      int
	Finished                      bool
	Heat                          int64
	Created                       time.Time
	Tags                          []string
}

type Episode struct {
	No          int
	ID, Title   string
	DurationSec int
}

type Play struct {
	URL, BackupURL string
	Key            []byte // nil when the episode is not encrypted
	Size           int64
	ExpiresAt      time.Time
}

type Hongguo interface {
	SearchComic(ctx context.Context, q, cursor string) (ids []string, next string, err error)
	Shows(ctx context.Context, ids []string) ([]Show, error)
	Episodes(ctx context.Context, seriesID string) ([]Episode, error)
	Related(ctx context.Context, seriesID string) ([]string, error)
	Resolve(ctx context.Context, episodeID, quality string) (*Play, error)
}

type sdkHongguo struct {
	c *sdk.Client
	r resetter

	// meta is a separate anonymous device used only for show metadata. It is
	// kept on a tag taxonomy that carries art style (see findStyledDevice).
	meta     *sdk.Client
	metaR    resetter
	metaMu   sync.Mutex
	metaGood bool

	// play is a separate anonymous device for ResolvePlayback: Hongguo serves
	// a fresh device exactly one playback, so it is replaced after every use.
	play     *sdk.Client
	playMu   sync.Mutex
	playUsed bool
}

// styleProbeID is a long-running 沙雕-style series used to check a device's taxonomy.
const styleProbeID = "7527216662647426110"

func NewSDK(dataDir string) (Hongguo, error) {
	c, err := adapters.NewClient(filepath.Join(dataDir, "anonymous.json"), sdk.Options{RetryAttempts: 3})
	if err != nil {
		return nil, err
	}
	meta, err := adapters.NewClient(filepath.Join(dataDir, "meta.json"), sdk.Options{RetryAttempts: 3})
	if err != nil {
		return nil, err
	}
	play, err := adapters.NewClient(filepath.Join(dataDir, "play.json"), sdk.Options{RetryAttempts: 3})
	if err != nil {
		return nil, err
	}
	return &sdkHongguo{c: c, meta: meta, play: play, playUsed: true}, nil
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("upstream: %w", err)
}

func (h *sdkHongguo) SearchComic(ctx context.Context, q, cursor string) ([]string, string, error) {
	var page *sdk.Page[model.SeriesSummary]
	err := h.r.do(ctx, func() (err error) {
		page, err = h.c.Search(ctx, q, sdk.SearchOptions{Cursor: cursor, Filters: []string{sdk.SearchFilterComic}})
		return err
	}, h.c.ResetDevice)
	if err != nil {
		return nil, "", wrap(err)
	}
	ids := make([]string, 0, len(page.Items))
	for _, s := range page.Items {
		ids = append(ids, s.ID)
	}
	next := ""
	if page.HasMore {
		next = page.Cursor
	}
	return ids, next, nil
}

func (h *sdkHongguo) Shows(ctx context.Context, ids []string) ([]Show, error) {
	if err := h.ensureStyledMeta(ctx); err != nil {
		return nil, err
	}
	var details []model.SeriesDetail
	err := h.metaR.do(ctx, func() (err error) {
		details, err = h.meta.GetSeriesBatch(ctx, ids)
		return err
	}, h.resetMeta)
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]Show, 0, len(details))
	for _, d := range details {
		if d.Series != nil {
			out = append(out, toShow(d.Series))
		}
	}
	return out, nil
}

func (h *sdkHongguo) resetMeta(ctx context.Context) error {
	h.metaMu.Lock()
	h.metaGood = false
	h.metaMu.Unlock()
	if err := h.meta.ResetDevice(ctx); err != nil {
		return err
	}
	return h.ensureStyledMeta(ctx)
}

func (h *sdkHongguo) ensureStyledMeta(ctx context.Context) error {
	h.metaMu.Lock()
	defer h.metaMu.Unlock()
	if h.metaGood {
		return nil
	}
	probe := func(ctx context.Context) ([]string, error) {
		ds, err := h.meta.GetSeriesBatch(ctx, []string{styleProbeID})
		if err != nil || len(ds) == 0 || ds[0].Series == nil {
			return nil, err
		}
		return toShow(ds[0].Series).Tags, nil
	}
	if err := findStyledDevice(ctx, probe, h.meta.ResetDevice, 12); err != nil {
		return wrap(err)
	}
	h.metaGood = true
	return nil
}

func (h *sdkHongguo) Episodes(ctx context.Context, seriesID string) ([]Episode, error) {
	var page *sdk.Page[model.Episode]
	err := h.r.do(ctx, func() (err error) {
		page, err = h.c.ListEpisodes(ctx, seriesID, sdk.EpisodeOptions{})
		return err
	}, h.c.ResetDevice)
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]Episode, 0, len(page.Items))
	for _, e := range page.Items {
		out = append(out, Episode{No: e.Number, ID: e.ID, Title: e.Title, DurationSec: int(e.Duration / time.Second)})
	}
	return out, nil
}

func (h *sdkHongguo) Related(ctx context.Context, seriesID string) ([]string, error) {
	rec, err := h.c.SeriesRecommend(ctx, seriesID)
	if err != nil {
		return nil, wrap(err)
	}
	ids := make([]string, 0, len(rec.Items))
	for _, s := range rec.Items {
		ids = append(ids, s.ID)
	}
	return ids, nil
}

func (h *sdkHongguo) Resolve(ctx context.Context, episodeID, quality string) (*Play, error) {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	if h.playUsed {
		if err := h.play.ResetDevice(ctx); err != nil {
			return nil, wrap(err)
		}
		h.playUsed = false
	}
	resolve := func() (pb *model.Playback, err error) {
		pb, err = h.play.ResolvePlayback(ctx, episodeID, sdk.PlaybackOptions{})
		if errors.Is(err, sdk.ErrDRMProtected) {
			err = nil
		}
		return pb, err
	}
	pb, err := resolve()
	if limited(err) {
		log.Printf("upstream: play device: %v; registering a new one", err)
		if err = h.play.ResetDevice(ctx); err == nil {
			pb, err = resolve()
		}
	}
	h.playUsed = true
	go h.renewPlayDevice()
	if err != nil {
		return nil, wrap(err)
	}
	st, err := pb.Select(quality)
	if err != nil {
		return nil, wrap(err)
	}
	p := &Play{URL: st.URL, BackupURL: st.BackupURL, Size: st.Size, ExpiresAt: time.Unix(pb.ExpiresAt, 0)}
	if pb.Protected {
		k, err := sdk.ContentKey(pb)
		if err != nil {
			return nil, wrap(err)
		}
		if p.Key, err = hex.DecodeString(k.Hex); err != nil {
			return nil, wrap(err)
		}
	}
	return p, nil
}

// renewPlayDevice registers the next play device ahead of time so the next
// Resolve doesn't wait for it.
func (h *sdkHongguo) renewPlayDevice() {
	h.playMu.Lock()
	defer h.playMu.Unlock()
	if !h.playUsed {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := h.play.ResetDevice(ctx); err == nil {
		h.playUsed = false
	}
}

func toShow(s *model.Series) Show {
	out := Show{
		ID: s.ID, Title: s.Title, Description: s.Description, Cover: s.CoverURL,
		Episodes: s.EpisodeCount, Finished: s.Status == model.SeriesEnd,
		Heat: s.Heat, Created: s.CreatedAt,
	}
	for _, r := range s.Rows {
		if r.Kind == model.InfoCategory {
			out.Tags = append(out.Tags, r.Text)
		}
	}
	return out
}
