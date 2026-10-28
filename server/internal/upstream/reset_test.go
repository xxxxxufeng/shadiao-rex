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
