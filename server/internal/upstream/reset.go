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
			return errors.New("upstream: no anonymous device with style tags")
		}
		if err := reset(ctx); err != nil {
			return err
		}
	}
}
