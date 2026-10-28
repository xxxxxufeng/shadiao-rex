package cenc

import (
	"encoding/binary"
	"testing"
)

func TestOversizedChildBox(t *testing.T) {
	trak := make([]byte, 8)
	binary.BigEndian.PutUint32(trak, 1000) // claims more than its parent holds
	copy(trak[4:], "trak")
	data := append(box("ftyp", []byte("isom0000")), box("moov", trak)...)
	if _, err := ParseAndRewrite(data); err == nil {
		t.Fatal("expected error")
	}
}

func TestHugeLargesize(t *testing.T) {
	trak := make([]byte, 16)
	binary.BigEndian.PutUint32(trak, 1)
	copy(trak[4:], "trak")
	binary.BigEndian.PutUint64(trak[8:], 1<<63-1)
	data := append(box("ftyp", []byte("isom0000")), box("moov", trak)...)
	if _, err := ParseAndRewrite(data); err == nil {
		t.Fatal("expected error")
	}
}

func TestTruncatedTables(t *testing.T) {
	// a protected track whose sample tables are empty boxes
	tenc := box("tenc", make([]byte, 24))
	tenc[8+7] = 8
	sinf := box("sinf", box("frma", []byte("hvc1")), box("schi", tenc))
	encv := box("encv", make([]byte, 78), sinf)
	stsd := box("stsd", []byte{0, 0, 0, 0, 0, 0, 0, 1}, encv)
	for _, tables := range [][]byte{
		append(append(box("stsz"), box("stco")...), box("stsc")...),
		append(append(box("stsz", make([]byte, 12)), box("stco", []byte{0, 0, 0, 0})...), box("stsc")...),
		box("stsz", []byte{0, 0, 0, 0, 0, 0, 0, 1, 0xff, 0xff, 0xff, 0xff}), // fixed size, 4G samples
	} {
		stbl := box("stbl", stsd, tables)
		moov := box("moov", box("trak", box("mdia", box("minf", stbl))))
		data := append(box("ftyp", []byte("isom0000")), moov...)
		if _, err := ParseAndRewrite(data); err == nil {
			t.Fatal("expected error")
		}
	}
}

func FuzzParseAndRewrite(f *testing.F) {
	f.Add(append(box("ftyp", []byte("isom0000")), box("moov", box("trak", box("mdia")))...))
	f.Fuzz(func(t *testing.T, data []byte) {
		l, err := ParseAndRewrite(data)
		if err == nil {
			l.DecryptAt(make([]byte, 16), 0, make([]byte, 64))
		}
	})
}
