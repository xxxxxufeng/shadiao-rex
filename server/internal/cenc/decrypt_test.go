package cenc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"math/rand"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// decPacketHashes hashes every packet of the ffmpeg-decrypted fixture.
func decPacketHashes(t *testing.T, dec []byte) []string {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "packet=pos,size", "-of", "csv=p=0", "../../testdata/ep.dec.mp4").Output()
	if err != nil {
		t.Fatal(err)
	}
	var hs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, ",")
		size, _ := strconv.Atoi(f[0])
		pos, _ := strconv.Atoi(f[1])
		h := sha256.Sum256(dec[pos : pos+size])
		hs = append(hs, string(h[:]))
	}
	sort.Strings(hs)
	return hs
}

func sampleHashes(l *Layout, buf []byte) []string {
	var hs []string
	for _, s := range l.Samples {
		h := sha256.Sum256(buf[s.Offset : s.Offset+s.Size])
		hs = append(hs, string(h[:]))
	}
	sort.Strings(hs)
	return hs
}

func TestDecryptFullMatchesFFmpeg(t *testing.T) {
	enc, dec, key := fixture(t)
	l, err := ParseAndRewrite(enc)
	if err != nil {
		t.Fatal(err)
	}
	buf := append([]byte(nil), enc...)
	l.DecryptAt(key, 0, buf)
	got, want := sampleHashes(l, buf), decPacketHashes(t, dec)
	if len(got) != len(want) {
		t.Fatalf("%d samples vs %d packets", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("sample hash mismatch at sorted index %d", i)
		}
	}
	if !bytes.Equal(buf[:l.MoovEnd], l.Header) {
		t.Fatal("header region not replaced")
	}
}

func TestDecryptRangeMatchesFull(t *testing.T) {
	enc, _, key := fixture(t)
	l, err := ParseAndRewrite(enc)
	if err != nil {
		t.Fatal(err)
	}
	full := append([]byte(nil), enc...)
	l.DecryptAt(key, 0, full)
	rng := rand.New(rand.NewSource(1))
	starts := []int64{0, 27, 1000, 134000, 134264, 134265, 134273, 134281, int64(len(enc)) - 5}
	for i := 0; i < 20; i++ {
		starts = append(starts, rng.Int63n(int64(len(enc))))
	}
	for _, st := range starts {
		n := int64(rng.Intn(300000) + 1)
		if st+n > int64(len(enc)) {
			n = int64(len(enc)) - st
		}
		part := append([]byte(nil), enc[st:st+n]...)
		l.DecryptAt(key, st, part)
		if !bytes.Equal(part, full[st:st+n]) {
			t.Fatalf("range [%d,+%d) differs", st, n)
		}
	}
}

func TestSubsampleCTR(t *testing.T) {
	key := []byte("0123456789abcdef")
	iv := []byte{9, 8, 7, 6, 5, 4, 3, 2}
	plain := make([]byte, 100)
	for i := range plain {
		plain[i] = byte(i)
	}
	subs := []Subsample{{10, 37}, {5, 48}}
	// encrypt protected ranges as one continuous CTR stream
	blk, _ := aes.NewCipher(key)
	ctr := make([]byte, 16)
	copy(ctr, iv)
	stream := cipher.NewCTR(blk, ctr)
	enc := append([]byte(nil), plain...)
	p := 0
	for _, s := range subs {
		p += int(s.Clear)
		stream.XORKeyStream(enc[p:p+int(s.Protected)], enc[p:p+int(s.Protected)])
		p += int(s.Protected)
	}
	l := &Layout{MoovEnd: 0, Samples: []Sample{{Offset: 1000, Size: 100, IV: iv, Subs: subs}}}
	for _, st := range []int{0, 11, 20, 47, 50, 99} {
		part := append([]byte(nil), enc[st:]...)
		l.DecryptAt(key, 1000+int64(st), part)
		if !bytes.Equal(part, plain[st:]) {
			t.Fatalf("start %d mismatch", st)
		}
	}
}
