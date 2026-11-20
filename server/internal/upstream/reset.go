package upstream

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/beilunyang/openhongguo/sdk"
)

// resetter re-registers the anonymous device when Hongguo stops serving it
// (empty responses / rejected device) and retries the call once. Resets are
// at most one per minute so concurrent requests don't churn devices.
type resetter struct {
	mu   sync.Mutex
	last time.Time
	now  func() time.Time
}

func limited(err error) bool {
	return errors.Is(err, sdk.ErrAnonymousLimited) || errors.Is(err, sdk.ErrDeviceRejected)
}

func (r *resetter) do(ctx context.Context, call func() error, reset func(context.Context) error) error {
	err := call()
	if !limited(err) {
		return err
	}
	now := time.Now
	if r.now != nil {
		now = r.now
	}
	r.mu.Lock()
	if r.last.IsZero() || now().Sub(r.last) >= time.Minute {
		r.last = now()
		log.Printf("upstream: %v; registering a new anonymous device", err)
		if rerr := reset(ctx); rerr != nil {
			r.mu.Unlock()
			return err
		}
	}
	r.mu.Unlock()
	return call()
}

// styled reports whether tags come from a taxonomy that carries art style.
func styled(tags []string) bool {
	for _, t := range tags {
		switch t {
		case "沙雕", "表情包", "3D", "3D漫":
			return true
		}
	}
	return false
}

// findStyledDevice re-registers the device until probe returns styled tags.
func findStyledDevice(ctx context.Context, probe func(context.Context) ([]string, error), reset func(context.Context) error, tries int) error {
	for i := 0; ; i++ {
		tags, err := probe(ctx)
		if err == nil && styled(tags) {
			return nil
		}
		if i == tries {
			return ErrNoStyledDevice
		}
		if err := reset(ctx); err != nil {
			return err
		}
	}
}

// styleBackoff is how long a failed search for a styled device is remembered.
// It matches the default pool refresh interval, so in practice the background
// refresh is what searches again, not someone waiting on a detail page.
const styleBackoff = 6 * time.Hour

// ErrNoStyledDevice: Hongguo is not handing out devices whose tags carry art style.
var ErrNoStyledDevice = errors.New("upstream: no anonymous device with style tags")

// styleGate remembers whether the meta device is on a styled taxonomy. When a
// search for one fails, it fails fast for styleBackoff instead of searching
// again on every request: Hongguo can stop handing out styled devices for
// days, and each search costs a dozen registrations and about ten seconds.
type styleGate struct {
	mu          sync.Mutex
	good        bool
	failedUntil time.Time
	now         func() time.Time
}

func (g *styleGate) invalidate() {
	g.mu.Lock()
	g.good = false
	g.mu.Unlock()
}

func (g *styleGate) ensure(ctx context.Context, probe func(context.Context) ([]string, error), reset func(context.Context) error, tries int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.good {
		return nil
	}
	now := time.Now
	if g.now != nil {
		now = g.now
	}
	if now().Before(g.failedUntil) {
		return ErrNoStyledDevice
	}
	if err := findStyledDevice(ctx, probe, reset, tries); err != nil {
		if ctx.Err() == nil {
			g.failedUntil = now().Add(styleBackoff)
			log.Printf("upstream: %v; not searching again for %s", err, styleBackoff)
		}
		return err
	}
	g.good = true
	return nil
}
