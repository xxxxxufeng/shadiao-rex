package upstream

import (
	"context"
	"testing"
	"time"

	"github.com/beilunyang/openhongguo/sdk"
)

func TestRetryAfterReset(t *testing.T) {
	r := &resetter{}
	resets, calls := 0, 0
	reset := func(context.Context) error { resets++; return nil }
	call := func() error {
		calls++
		if resets == 0 {
			return sdk.ErrAnonymousLimited
		}
		return nil
	}
	if err := r.do(context.Background(), call, reset); err != nil {
		t.Fatal(err)
	}
	if resets != 1 || calls != 2 {
		t.Fatalf("resets %d calls %d", resets, calls)
	}
}

func TestNoResetForOtherErrors(t *testing.T) {
	r := &resetter{}
	resets := 0
	err := r.do(context.Background(), func() error { return sdk.ErrContentUnavailable }, func(context.Context) error { resets++; return nil })
	if err == nil || resets != 0 {
		t.Fatalf("err %v resets %d", err, resets)
	}
}

func TestResetRateLimited(t *testing.T) {
	r := &resetter{now: func() time.Time { return time.Unix(1000, 0) }}
	resets := 0
	limited := func() error { return sdk.ErrDeviceRejected }
	reset := func(context.Context) error { resets++; return nil }
	r.do(context.Background(), limited, reset)
	r.do(context.Background(), limited, reset) // same instant: no second reset
	if resets != 1 {
		t.Fatalf("resets %d", resets)
	}
}

func TestFindStyledDevice(t *testing.T) {
	probes := [][]string{{"恋爱", "搞笑"}, {"宫廷", "搞笑"}, {"喜剧", "沙雕"}}
	i, resets := 0, 0
	probe := func(context.Context) ([]string, error) { tags := probes[min(i, len(probes)-1)]; i++; return tags, nil }
	reset := func(context.Context) error { resets++; return nil }
	if err := findStyledDevice(context.Background(), probe, reset, 12); err != nil {
		t.Fatal(err)
	}
	if resets != 2 {
		t.Fatalf("resets %d", resets)
	}
	i, resets = 0, 0
	probes = [][]string{{"恋爱"}}
	if err := findStyledDevice(context.Background(), probe, reset, 3); err == nil || resets != 3 {
		t.Fatalf("err %v resets %d", err, resets)
	}
}

func TestSplitProbe(t *testing.T) {
	a := Show{ID: "a", Tags: []string{"喜剧"}}
	probe := func(tags ...string) Show { return Show{ID: styleProbeID, Tags: tags} }

	out, drifted := splitProbe([]Show{a, probe("喜剧", "沙雕")}, true)
	if len(out) != 1 || out[0].ID != "a" || drifted {
		t.Fatalf("styled probe added by us: %v %v", out, drifted)
	}
	out, drifted = splitProbe([]Show{a, probe("喜剧", "表情包")}, false)
	if len(out) != 2 || drifted {
		t.Fatalf("styled probe the caller asked for stays: %v %v", out, drifted)
	}
	if _, drifted = splitProbe([]Show{a, probe("喜剧", "搞笑")}, true); !drifted {
		t.Fatal("unstyled probe means the device drifted")
	}
	if _, drifted = splitProbe([]Show{a}, true); drifted {
		t.Fatal("missing probe is unknown, not drift")
	}
}

// A failed search for a styled device must not be repeated on every request:
// each one costs a dozen device registrations and ~10s.
func TestStyleGateBacksOffAfterFailedSearch(t *testing.T) {
	now := time.Unix(1000, 0)
	g := &styleGate{now: func() time.Time { return now }}
	probes, resets := 0, 0
	tags := []string{"恋爱"}
	probe := func(context.Context) ([]string, error) { probes++; return tags, nil }
	reset := func(context.Context) error { resets++; return nil }
	ctx := context.Background()

	if err := g.ensure(ctx, probe, reset, 3); err == nil {
		t.Fatal("want error when no device is styled")
	}
	if resets != 3 {
		t.Fatalf("resets %d", resets)
	}
	if err := g.ensure(ctx, probe, reset, 3); err == nil || resets != 3 || probes != 4 {
		t.Fatalf("inside the back-off: err %v resets %d probes %d", err, resets, probes)
	}

	now = now.Add(styleBackoff)
	tags = []string{"沙雕"}
	if err := g.ensure(ctx, probe, reset, 3); err != nil {
		t.Fatalf("after the back-off: %v", err)
	}
	if err := g.ensure(ctx, probe, reset, 3); err != nil || probes != 5 {
		t.Fatalf("a good device is not probed again: err %v probes %d", err, probes)
	}
	g.invalidate()
	if err := g.ensure(ctx, probe, reset, 3); err != nil || probes != 6 {
		t.Fatalf("invalidate forces a new probe: err %v probes %d", err, probes)
	}
}

// A search cut short by its caller going away says nothing about the devices.
func TestStyleGateNoBackoffWhenCanceled(t *testing.T) {
	g := &styleGate{}
	ctx, cancel := context.WithCancel(context.Background())
	probe := func(context.Context) ([]string, error) { return nil, nil }
	reset := func(context.Context) error { cancel(); return ctx.Err() }
	if err := g.ensure(ctx, probe, reset, 3); err == nil {
		t.Fatal("want error")
	}
	if !g.failedUntil.IsZero() {
		t.Fatal("a canceled search must not start the back-off")
	}
}
