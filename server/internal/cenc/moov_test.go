package cenc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFixture(t *testing.T) {
	enc, _, _ := fixture(t)
	l, err := ParseAndRewrite(enc)
	if err != nil {
		t.Fatal(err)
	}
	if l.MoovStart != 28 || l.MoovEnd != 134265 {
		t.Fatalf("moov [%d,%d)", l.MoovStart, l.MoovEnd)
	}
	if len(l.Samples) != 3206+2305 {
		t.Fatalf("samples %d", len(l.Samples))
	}
	for i, s := range l.Samples {
		if len(s.IV) != 8 || len(s.Subs) != 0 {
			t.Fatalf("sample %d iv %d subs %d", i, len(s.IV), len(s.Subs))
		}
		if i > 0 && s.Offset < l.Samples[i-1].Offset+l.Samples[i-1].Size {
			t.Fatalf("sample %d not sorted/non-overlapping", i)
		}
	}
}

func TestNeedHead(t *testing.T) {
	enc, _, _ := fixture(t)
	n, err := NeedHead(enc[:64])
	if err != nil || n != 134265 {
		t.Fatalf("need %d err %v", n, err)
	}
}

func TestRewriteRemovesEncryption(t *testing.T) {
	enc, _, _ := fixture(t)
	l, err := ParseAndRewrite(enc[:200000])
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(l.Header)) != l.MoovEnd {
		t.Fatalf("header len %d", len(l.Header))
	}
	for _, s := range []string{"encv", "enca", "sinf", "senc", "saiz", "saio"} {
		if bytes.Contains(l.Header, []byte(s)) {
			t.Fatalf("header still contains %s", s)
		}
	}
	if !bytes.Equal(l.Header[:28], enc[:28]) {
		t.Fatal("ftyp changed")
	}
	p := filepath.Join(t.TempDir(), "x.mp4")
	os.WriteFile(p, append(append([]byte{}, l.Header...), enc[l.MoovEnd:]...), 0o644)
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_tag_string", "-of", "csv=p=0", p).Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "hvc1") || !strings.Contains(string(out), "mp4a") {
		t.Fatalf("ffprobe: %s", out)
	}
}

func box(typ string, body ...[]byte) []byte {
	b := bytes.Join(body, nil)
	out := make([]byte, 8, 8+len(b))
	binary.BigEndian.PutUint32(out, uint32(8+len(b)))
	copy(out[4:], typ)
	return append(out, b...)
}

func TestNeedHeadMoovAtEnd(t *testing.T) {
	data := append(box("ftyp", []byte("isom0000")), box("mdat", make([]byte, 100))...)
	data = append(data, box("moov", box("mvhd", make([]byte, 100)))...)
	if _, err := NeedHead(data); !errors.Is(err, ErrMoovAtEnd) {
		t.Fatalf("err %v", err)
	}
}

func TestSubsampleParse(t *testing.T) {
	// senc: version/flags=2, count=2; each: iv(8) + subcount(2) + n*(clear u16, prot u32)
	var b bytes.Buffer
	b.Write([]byte{0, 0, 0, 2, 0, 0, 0, 2})
	for i := 0; i < 2; i++ {
		b.Write([]byte{1, 2, 3, 4, 5, 6, 7, byte(i)})
		b.Write([]byte{0, 2})
		b.Write([]byte{0, 5, 0, 0, 0, 16})
		b.Write([]byte{0, 3, 0, 0, 0, 32})
	}
	ivs, subs, err := parseSenc(b.Bytes(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(ivs) != 2 || ivs[1][7] != 1 {
		t.Fatalf("ivs %v", ivs)
	}
	want := []Subsample{{5, 16}, {3, 32}}
	if len(subs[0]) != 2 || subs[0][0] != want[0] || subs[1][1] != want[1] {
		t.Fatalf("subs %v", subs)
	}
}
