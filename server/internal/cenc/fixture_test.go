package cenc

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T) (enc, dec, key []byte) {
	t.Helper()
	var err error
	if enc, err = os.ReadFile("../../testdata/ep.mp4"); err != nil {
		t.Skip("fixture missing: see testdata/README.md")
	}
	if dec, err = os.ReadFile("../../testdata/ep.dec.mp4"); err != nil {
		t.Skip("fixture missing")
	}
	k, err := os.ReadFile("../../testdata/ep.key")
	if err != nil {
		t.Skip("fixture missing")
	}
	if key, err = hex.DecodeString(strings.TrimSpace(string(k))); err != nil {
		t.Fatal(err)
	}
	return
}
