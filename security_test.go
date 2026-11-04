// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
)

// pngWithRawProfileIPTC builds a minimal PNG with a single zTXt chunk carrying the
// "Raw profile type iptc" keyword. declaredDataLen goes into the chunk length, so it
// can be made to disagree with the payload actually delivered.
func pngWithRawProfileIPTC(declaredDataLen int, payload []byte) []byte {
	keyword := append([]byte("Raw profile type iptc"), 0)
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	binary.Write(&buf, binary.BigEndian, uint32(len(keyword)+declaredDataLen))
	buf.WriteString("zTXt")
	buf.Write(keyword)
	buf.Write(payload)
	buf.Write([]byte{0, 0, 0, 0}) // CRC
	return buf.Bytes()
}

// zTXtPayload returns a deflate compression method byte followed by a zlib stream
// that inflates to size bytes of hex digits.
func zTXtPayload(size int) []byte {
	var buf bytes.Buffer
	buf.WriteByte(0) // Compression method: deflate.
	w := zlib.NewWriter(&buf)
	w.Write(bytes.Repeat([]byte("41"), size/2))
	w.Close()
	return buf.Bytes()
}

func decodePNGIPTC(b []byte) error {
	_, err := Decode(Options{
		R:           bytes.NewReader(b),
		ImageFormat: PNG,
		Sources:     IPTC,
	})
	return err
}

// A length read off the wire must never reach make. This is the bound the eXIf path
// has always had via bufferedReader; it lives in the shared read primitive so every
// size-bearing read inherits it.
func TestReadNBounds(t *testing.T) {
	c := qt.New(t)

	read := func(n int) error {
		e := newStreamReader(bytes.NewReader(make([]byte, 8)), binary.BigEndian)
		return e.readNFromRIntoBufE(n, e.r)
	}

	c.Assert(read(maxBufSize+1), qt.ErrorMatches, ".*exceeds max.*")
	c.Assert(read(-1), qt.ErrorMatches, ".*negative length.*")
	c.Assert(read(4), qt.IsNil)
}

// A small zlib stream must not be allowed to inflate without bound.
func TestDecompressZTXtBounds(t *testing.T) {
	c := qt.New(t)

	_, err := decompressZTXt(zTXtPayload(maxBufSize * 4))
	c.Assert(err, qt.ErrorMatches, ".*uncompressed zTXt size exceeds max.*")

	_, err = decompressZTXt(nil)
	c.Assert(err, qt.ErrorMatches, ".*no zTXt data.*")

	b, err := decompressZTXt(zTXtPayload(64))
	c.Assert(err, qt.IsNil)
	c.Assert(len(b), qt.Equals, 64)
}

// End to end: a crafted PNG must not be able to drive an allocation from its declared
// chunk length, nor inflate without bound. Both are memory exhaustion, which recover
// cannot catch.
func TestDecodePNGZTXtRawProfileIPTC(t *testing.T) {
	c := qt.New(t)

	c.Run("inflated size beyond max", func(c *qt.C) {
		payload := zTXtPayload(maxBufSize * 4)
		c.Assert(len(payload) < 600*1024, qt.IsTrue, qt.Commentf("payload is %d bytes", len(payload)))
		c.Assert(decodePNGIPTC(pngWithRawProfileIPTC(len(payload), payload)), qt.ErrorMatches, ".*uncompressed zTXt size exceeds max.*")
	})

	c.Run("inflated size shorter than keyword", func(c *qt.C) {
		payload := zTXtPayload(4)
		c.Assert(decodePNGIPTC(pngWithRawProfileIPTC(len(payload), payload)), qt.ErrorMatches, ".*zTXt data too short.*")
	})

	c.Run("no data", func(c *qt.C) {
		c.Assert(decodePNGIPTC(pngWithRawProfileIPTC(0, nil)), qt.ErrorMatches, ".*no zTXt data.*")
	})

	// 42 bytes on disk claiming 256 MB of zTXt data. The declared length is rejected
	// before it reaches make, so this returns without consuming the claimed memory.
	c.Run("declared length beyond max", func(c *qt.C) {
		png := pngWithRawProfileIPTC(256<<20, nil)
		c.Assert(len(png) < 64, qt.IsTrue)
		c.Assert(decodePNGIPTC(png), qt.IsNil)
	})
}

// resolveCodedCharacterSet indexes b[4], so it must require 5 bytes, not 4.
func TestResolveCodedCharacterSetShortInput(t *testing.T) {
	c := qt.New(t)

	for i := range 6 {
		b := bytes.Repeat([]byte{0x1b}, i)
		c.Assert(func() { resolveCodedCharacterSet(b) }, qt.Not(qt.PanicMatches), ".*")
	}

	c.Assert(resolveCodedCharacterSet([]byte{0x1b, 0x2e, 0x2e, 0x2e}), qt.Equals, "")
	c.Assert(resolveCodedCharacterSet([]byte{0x1b, 0x2e, 0x2e, 0x2e, 0x41}), qt.Equals, characterSetISO88591)
	c.Assert(resolveCodedCharacterSet([]byte{0x1b, 0x25, 0x47}), qt.Equals, characterSetUTF8)
}

// tiffWithSubIFDFanOut builds a little endian TIFF whose IFD0 holds a single SubIFDs
// tag with numOffsets offsets all pointing to the same IFD of numTags oversized tags.
func tiffWithSubIFDFanOut(numOffsets, numTags int) []byte {
	le := binary.LittleEndian
	const ifd0Off = 8
	arrOff := ifd0Off + 2 + 12 + 4
	nestedOff := arrOff + 4*numOffsets

	var buf bytes.Buffer
	buf.WriteString("II")
	binary.Write(&buf, le, uint16(42))
	binary.Write(&buf, le, uint32(ifd0Off))

	binary.Write(&buf, le, uint16(1))
	binary.Write(&buf, le, uint16(0x014a)) // SubIFDs
	binary.Write(&buf, le, uint16(4))      // LONG
	binary.Write(&buf, le, uint32(numOffsets))
	binary.Write(&buf, le, uint32(arrOff))
	binary.Write(&buf, le, uint32(0))

	for range numOffsets {
		binary.Write(&buf, le, uint32(nestedOff))
	}

	binary.Write(&buf, le, uint16(numTags))
	for i := range numTags {
		binary.Write(&buf, le, uint16(0xc000+i%0x1000))
		binary.Write(&buf, le, uint16(3)) // SHORT
		binary.Write(&buf, le, uint32(6000))
		binary.Write(&buf, le, uint32(0))
	}
	binary.Write(&buf, le, uint32(0))
	return buf.Bytes()
}

// Tags skipped for size and IFD pointers must count against LimitNumTags,
// or a SubIFD fan-out can make a small file decode for minutes.
func TestSubIFDFanOutLimitNumTags(t *testing.T) {
	c := qt.New(t)

	b := tiffWithSubIFDFanOut(2500, 65535)

	for _, f := range []ImageFormat{TIFF, DNG} {
		for _, sources := range []Source{0, CONFIG} {
			_, err := Decode(Options{
				R:           bytes.NewReader(b),
				ImageFormat: f,
				Sources:     sources,
				Timeout:     5 * time.Second,
			})
			c.Assert(err, qt.IsNil, qt.Commentf("%s %d", f, sources))
		}
	}
}
