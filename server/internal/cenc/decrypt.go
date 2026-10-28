package cenc

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"sort"
)

// DecryptAt decrypts buf in place, where buf holds the original file bytes
// [fileOffset, fileOffset+len(buf)). Bytes inside [0, MoovEnd) are replaced by
// the rewritten Header.
func (l *Layout) DecryptAt(key []byte, fileOffset int64, buf []byte) {
	end := fileOffset + int64(len(buf))
	if fileOffset < l.MoovEnd {
		copy(buf, l.Header[fileOffset:min(l.MoovEnd, end)])
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	i := sort.Search(len(l.Samples), func(i int) bool {
		s := l.Samples[i]
		return s.Offset+s.Size > fileOffset
	})
	for ; i < len(l.Samples) && l.Samples[i].Offset < end; i++ {
		decryptSample(blk, l.Samples[i], fileOffset, buf)
	}
}

// decryptSample decrypts the part of sample s that overlaps buf.
func decryptSample(blk cipher.Block, s Sample, fileOffset int64, buf []byte) {
	subs := s.Subs
	if len(subs) == 0 {
		subs = []Subsample{{0, uint32(s.Size)}}
	}
	pos := s.Offset  // file position of the current subsample segment
	var ctrOff int64 // offset into the sample's protected byte stream
	bufEnd := fileOffset + int64(len(buf))
	for _, sub := range subs {
		pos += int64(sub.Clear)
		protEnd := pos + int64(sub.Protected)
		from, to := max(pos, fileOffset), min(protEnd, bufEnd)
		if from < to {
			xorKeystream(blk, s.IV, ctrOff+(from-pos), buf[from-fileOffset:to-fileOffset])
		}
		ctrOff += int64(sub.Protected)
		pos = protEnd
		if pos >= bufEnd {
			return
		}
	}
}

// xorKeystream XORs data with the AES-CTR keystream starting at byte p of the
// stream whose first counter block is IV || 0.
func xorKeystream(blk cipher.Block, iv []byte, p int64, data []byte) {
	var ctr [16]byte
	copy(ctr[:], iv)
	if len(iv) == 8 {
		binary.BigEndian.PutUint64(ctr[8:], uint64(p/16))
	} else {
		addCounter(&ctr, uint64(p/16))
	}
	stream := cipher.NewCTR(blk, ctr[:])
	if skip := int(p % 16); skip > 0 {
		var junk [16]byte
		stream.XORKeyStream(junk[:skip], junk[:skip])
	}
	stream.XORKeyStream(data, data)
}

// addCounter adds n to a full 128-bit big-endian counter (16-byte IVs).
func addCounter(ctr *[16]byte, n uint64) {
	lo := binary.BigEndian.Uint64(ctr[8:])
	sum := lo + n
	binary.BigEndian.PutUint64(ctr[8:], sum)
	if sum < lo {
		hi := binary.BigEndian.Uint64(ctr[:8])
		binary.BigEndian.PutUint64(ctr[:8], hi+1)
	}
}
