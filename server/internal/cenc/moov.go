// Package cenc turns a non-fragmented CENC ('cenc', AES-CTR) MP4 into a plain
// one on the fly: it neutralizes the encryption boxes in moov without changing
// any box size, and decrypts sample bytes at arbitrary file offsets.
package cenc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrMoovAtEnd = errors.New("cenc: moov after mdat is not supported")
	ErrShortHead = errors.New("cenc: head too short to locate moov")
	ErrBadBox    = errors.New("cenc: malformed box")
)

// maxSamples bounds per-track sample tables (a 2-minute episode has ~5k).
const maxSamples = 1 << 22

type Subsample struct {
	Clear     uint16
	Protected uint32
}

type Sample struct {
	Offset, Size int64
	IV           []byte
	Subs         []Subsample
}

type Layout struct {
	MoovStart, MoovEnd int64
	Header             []byte // file bytes [0, MoovEnd) with encryption boxes neutralized
	Samples            []Sample
}

type boxInfo struct {
	typ              string
	start, body, end int // offsets within the slice being walked
}

// walk lists the boxes in b[from:to]; it stops at a truncated box.
func walk(b []byte, from, to int) ([]boxInfo, error) {
	var out []boxInfo
	for off := from; off+8 <= to; {
		size := int(binary.BigEndian.Uint32(b[off:]))
		hdr := 8
		if size == 1 {
			if off+16 > to {
				return out, nil
			}
			big := binary.BigEndian.Uint64(b[off+8:])
			if big > uint64(to-off) {
				return nil, ErrBadBox
			}
			size = int(big)
			hdr = 16
		} else if size == 0 {
			size = to - off
		}
		if size < hdr || size > to-off {
			if from == 0 && to == len(b) && size >= hdr {
				// top level of a partial download: the last box may run past the head
				out = append(out, boxInfo{string(b[off+4 : off+8]), off, off + hdr, off + size})
				return out, nil
			}
			return nil, ErrBadBox
		}
		out = append(out, boxInfo{string(b[off+4 : off+8]), off, off + hdr, off + size})
		off += size
	}
	return out, nil
}

// NeedHead returns how many leading file bytes ParseAndRewrite needs (the end of moov).
func NeedHead(head []byte) (int64, error) {
	boxes, err := walk(head, 0, len(head))
	if err != nil {
		return 0, err
	}
	for _, bx := range boxes {
		switch bx.typ {
		case "moov":
			return int64(bx.end), nil
		case "mdat":
			return 0, ErrMoovAtEnd
		}
	}
	return 0, ErrShortHead
}

func ParseAndRewrite(head []byte) (*Layout, error) {
	moovEnd, err := NeedHead(head)
	if err != nil {
		return nil, err
	}
	if int64(len(head)) < moovEnd {
		return nil, ErrShortHead
	}
	hdr := append([]byte(nil), head[:moovEnd]...)
	top, _ := walk(hdr, 0, len(hdr))
	l := &Layout{MoovEnd: moovEnd, Header: hdr}
	for _, bx := range top {
		if bx.typ != "moov" {
			continue
		}
		l.MoovStart = int64(bx.start)
		traks, err := walk(hdr, bx.body, bx.end)
		if err != nil {
			return nil, err
		}
		for _, t := range traks {
			if t.typ != "trak" {
				continue
			}
			samples, err := parseTrak(hdr, t)
			if err != nil {
				return nil, err
			}
			l.Samples = append(l.Samples, samples...)
		}
	}
	sort.Slice(l.Samples, func(i, j int) bool { return l.Samples[i].Offset < l.Samples[j].Offset })
	return l, nil
}

func find(b []byte, parent boxInfo, path ...string) (boxInfo, bool) {
	cur := parent
	for _, name := range path {
		kids, err := walk(b, cur.body, cur.end)
		if err != nil {
			return boxInfo{}, false
		}
		found := false
		for _, k := range kids {
			if k.typ == name {
				cur, found = k, true
				break
			}
		}
		if !found {
			return boxInfo{}, false
		}
	}
	return cur, true
}

func neutralize(b []byte, bx boxInfo) { copy(b[bx.start+4:bx.start+8], "free") }

// parseTrak rewrites the trak's encryption boxes in place and returns its
// encrypted samples (nil for a clear track).
func parseTrak(b []byte, trak boxInfo) ([]Sample, error) {
	stbl, ok := find(b, trak, "mdia", "minf", "stbl")
	if !ok {
		return nil, nil
	}
	stsd, ok := find(b, stbl, "stsd")
	if !ok || stsd.body+8 > stsd.end {
		return nil, nil
	}
	entries, err := walk(b, stsd.body+8, stsd.end)
	if err != nil {
		return nil, err
	}
	ivSize := -1
	for _, e := range entries {
		var skip int
		switch e.typ {
		case "encv":
			skip = 78
		case "enca":
			skip = 28
		default:
			continue
		}
		kids, err := walk(b, e.body+skip, e.end)
		if err != nil {
			return nil, err
		}
		for _, k := range kids {
			if k.typ != "sinf" {
				continue
			}
			if frma, ok := find(b, k, "frma"); ok && frma.body+4 <= frma.end {
				copy(b[e.start+4:e.start+8], b[frma.body:frma.body+4])
			}
			if tenc, ok := find(b, k, "schi", "tenc"); ok && tenc.body+8 <= tenc.end {
				ivSize = int(b[tenc.body+7])
			}
			neutralize(b, k)
		}
	}
	if ivSize < 0 {
		return nil, nil
	}
	if ivSize == 0 {
		return nil, fmt.Errorf("cenc: constant IV not supported")
	}
	sizes, err := parseStsz(b, stbl)
	if err != nil {
		return nil, err
	}
	offsets, err := sampleOffsets(b, stbl, sizes)
	if err != nil {
		return nil, err
	}
	senc, ok := find(b, stbl, "senc")
	if !ok {
		return nil, fmt.Errorf("cenc: protected track without senc")
	}
	ivs, subs, err := parseSenc(b[senc.body:senc.end], ivSize)
	if err != nil {
		return nil, err
	}
	if len(ivs) != len(sizes) {
		return nil, fmt.Errorf("cenc: senc has %d entries for %d samples", len(ivs), len(sizes))
	}
	kids, _ := walk(b, stbl.body, stbl.end)
	for _, k := range kids {
		switch k.typ {
		case "senc", "saiz", "saio", "pssh", "sbgp", "sgpd":
			if k.typ == "sbgp" || k.typ == "sgpd" {
				// only seig sample groups describe encryption
				if k.body+12 > k.end || string(b[k.body+4:k.body+8]) != "seig" {
					continue
				}
			}
			neutralize(b, k)
		}
	}
	out := make([]Sample, len(sizes))
	for i := range sizes {
		out[i] = Sample{Offset: offsets[i], Size: sizes[i], IV: ivs[i]}
		if subs != nil {
			out[i].Subs = subs[i]
		}
	}
	return out, nil
}

func parseStsz(b []byte, stbl boxInfo) ([]int64, error) {
	bx, ok := find(b, stbl, "stsz")
	if !ok || bx.body+12 > bx.end {
		return nil, fmt.Errorf("cenc: missing stsz")
	}
	fixed := int64(binary.BigEndian.Uint32(b[bx.body+4:]))
	n := int(binary.BigEndian.Uint32(b[bx.body+8:]))
	if n > maxSamples || (fixed == 0 && bx.body+12+4*n > bx.end) {
		return nil, ErrBadBox
	}
	sizes := make([]int64, n)
	for i := range sizes {
		if fixed != 0 {
			sizes[i] = fixed
		} else {
			sizes[i] = int64(binary.BigEndian.Uint32(b[bx.body+12+4*i:]))
		}
	}
	return sizes, nil
}

func sampleOffsets(b []byte, stbl boxInfo, sizes []int64) ([]int64, error) {
	var chunks []int64
	if bx, ok := find(b, stbl, "stco"); ok {
		if bx.body+8 > bx.end {
			return nil, ErrBadBox
		}
		n := int(binary.BigEndian.Uint32(b[bx.body+4:]))
		if bx.body+8+4*n > bx.end {
			return nil, ErrBadBox
		}
		for i := 0; i < n; i++ {
			chunks = append(chunks, int64(binary.BigEndian.Uint32(b[bx.body+8+4*i:])))
		}
	} else if bx, ok := find(b, stbl, "co64"); ok {
		if bx.body+8 > bx.end {
			return nil, ErrBadBox
		}
		n := int(binary.BigEndian.Uint32(b[bx.body+4:]))
		if bx.body+8+8*n > bx.end {
			return nil, ErrBadBox
		}
		for i := 0; i < n; i++ {
			chunks = append(chunks, int64(binary.BigEndian.Uint64(b[bx.body+8+8*i:])))
		}
	} else {
		return nil, fmt.Errorf("cenc: missing stco/co64")
	}
	stsc, ok := find(b, stbl, "stsc")
	if !ok || stsc.body+8 > stsc.end {
		return nil, fmt.Errorf("cenc: missing stsc")
	}
	n := int(binary.BigEndian.Uint32(b[stsc.body+4:]))
	if stsc.body+8+12*n > stsc.end || n == 0 {
		return nil, ErrBadBox
	}
	type run struct{ first, per int }
	runs := make([]run, n)
	for i := range runs {
		p := stsc.body + 8 + 12*i
		runs[i] = run{int(binary.BigEndian.Uint32(b[p:])), int(binary.BigEndian.Uint32(b[p+4:]))}
	}
	offsets := make([]int64, 0, len(sizes))
	s := 0
	for ci := range chunks {
		chunkNo := ci + 1
		per := 0
		for _, r := range runs {
			if r.first <= chunkNo {
				per = r.per
			}
		}
		off := chunks[ci]
		for j := 0; j < per && s < len(sizes); j++ {
			offsets = append(offsets, off)
			off += sizes[s]
			s++
		}
	}
	if len(offsets) != len(sizes) {
		return nil, fmt.Errorf("cenc: chunk table covers %d of %d samples", len(offsets), len(sizes))
	}
	return offsets, nil
}

// parseSenc parses a senc box body (from version/flags on).
func parseSenc(body []byte, ivSize int) ([][]byte, [][]Subsample, error) {
	if len(body) < 8 {
		return nil, nil, ErrBadBox
	}
	flags := binary.BigEndian.Uint32(body) & 0xffffff
	n := int(binary.BigEndian.Uint32(body[4:]))
	if n > maxSamples || n*ivSize > len(body) {
		return nil, nil, ErrBadBox
	}
	p := 8
	ivs := make([][]byte, n)
	var subs [][]Subsample
	if flags&2 != 0 {
		subs = make([][]Subsample, n)
	}
	for i := 0; i < n; i++ {
		if p+ivSize > len(body) {
			return nil, nil, ErrBadBox
		}
		ivs[i] = body[p : p+ivSize : p+ivSize]
		p += ivSize
		if subs == nil {
			continue
		}
		if p+2 > len(body) {
			return nil, nil, ErrBadBox
		}
		m := int(binary.BigEndian.Uint16(body[p:]))
		p += 2
		if p+6*m > len(body) {
			return nil, nil, ErrBadBox
		}
		subs[i] = make([]Subsample, m)
		for j := range subs[i] {
			subs[i][j] = Subsample{binary.BigEndian.Uint16(body[p:]), binary.BigEndian.Uint32(body[p+2:])}
			p += 6
		}
	}
	return ivs, subs, nil
}
