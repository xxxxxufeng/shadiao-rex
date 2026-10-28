//go:build live

package upstream

import (
	"context"
	"testing"
)

func TestLiveStyledMeta(t *testing.T) {
	h, err := NewSDK(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	shows, err := h.Shows(context.Background(), []string{"7672595482966772760", "7527216662647426110"})
	if err != nil || len(shows) != 2 {
		t.Fatalf("%v %d", err, len(shows))
	}
	if !styled(shows[0].Tags) || !styled(shows[1].Tags) {
		t.Fatalf("unstyled tags: %v / %v", shows[0].Tags, shows[1].Tags)
	}
}
