// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/binary"
	"errors"
	"math"
)

type imageEncoderWebP struct {
	*baseEncoder
}

// encode converts a simple WebP file to the extended format (VP8X) if needed
// and appends EXIF and XMP chunks.
func (e *imageEncoderWebP) encode() error {
	if e.readFourCC() != webpFCC.riff {
		return newInvalidFormatErrorf("webp: invalid header")
	}
	riffSize := uint64(e.read4())
	if e.readFourCC() != webpFCC.webp {
		return newInvalidFormatErrorf("webp: invalid header")
	}
	if e.err != nil || riffSize < 12 || 8+riffSize > uint64(len(e.b)) {
		return newInvalidFormatErrorf("webp: invalid RIFF size")
	}
	e.b = e.b[:int(8+riffSize)]

	var (
		firstFCC  fourCC
		firstData []byte
		needPad   bool
	)
	for e.remaining() > 0 {
		chunkStart := e.pos
		if e.remaining() < 8 {
			return newInvalidFormatErrorf("webp: truncated chunk at offset %d", chunkStart)
		}
		chunkID := e.readFourCC()
		chunkLen := uint64(e.read4())
		if chunkLen > uint64(e.remaining()) {
			return newInvalidFormatErrorf("webp: invalid chunk length at offset %d", chunkStart)
		}
		data := e.readBytes(int(chunkLen))
		if chunkStart == 12 {
			firstFCC, firstData = chunkID, data
		}
		if chunkID == webpFCC.exif || chunkID == webpFCC.xmp {
			return errMetadataExistsf("webp: %s chunk at offset %d", chunkID, chunkStart)
		}
		if chunkLen%2 == 1 {
			if e.remaining() > 0 {
				e.skip(1)
			} else {
				needPad = true
			}
		}
	}

	exif := e.meta.encodeEXIF()
	flags := byte(webpFlagXMP)
	if exif != nil {
		flags |= webpFlagEXIF
	}

	var tail []byte
	if exif != nil {
		tail = appendWebPChunk(tail, webpFCC.exif, exif)
	}
	tail = appendWebPChunk(tail, webpFCC.xmp, e.meta.encodeXMP())

	var pad []byte
	if needPad {
		pad = []byte{0}
	}

	var riffHdr [12]byte
	copy(riffHdr[:4], webpFCC.riff[:])
	copy(riffHdr[8:12], webpFCC.webp[:])

	switch firstFCC {
	case webpFCC.vp8x:
		if len(firstData) != 10 {
			return newInvalidFormatErrorf("webp: invalid VP8X chunk length %d", len(firstData))
		}
		if firstData[0]&(webpFlagEXIF|webpFlagXMP) != 0 {
			return errMetadataExistsf("webp: VP8X flags %08b", firstData[0])
		}
		newRiffSize := uint64(len(e.b)-8) + uint64(len(pad)) + uint64(len(tail))
		if newRiffSize > math.MaxUint32 {
			return errors.New("webp: file too large")
		}
		e.byteOrder.PutUint32(riffHdr[4:], uint32(newRiffSize))
		var vp8xChunk [18]byte
		copy(vp8xChunk[:], e.b[12:30])
		vp8xChunk[8] |= flags
		return e.writeChunks(riffHdr[:], vp8xChunk[:], e.b[30:], pad, tail)
	case webpFCC.vp8, webpFCC.vp8l:
		var width, height uint32
		if firstFCC == webpFCC.vp8 {
			if len(firstData) < 10 || firstData[0]&1 != 0 || firstData[3] != 0x9d || firstData[4] != 0x01 || firstData[5] != 0x2a {
				return newInvalidFormatErrorf("webp: invalid VP8 frame header")
			}
			width = uint32(e.byteOrder.Uint16(firstData[6:]) & 0x3fff)
			height = uint32(e.byteOrder.Uint16(firstData[8:]) & 0x3fff)
			if width == 0 || height == 0 {
				return newInvalidFormatErrorf("webp: invalid VP8 dimensions")
			}
		} else {
			if len(firstData) < 5 || firstData[0] != 0x2f {
				return newInvalidFormatErrorf("webp: invalid VP8L signature")
			}
			bits := e.byteOrder.Uint32(firstData[1:])
			width = bits&0x3fff + 1
			height = (bits>>14)&0x3fff + 1
			if bits>>28&1 == 1 {
				flags |= webpFlagAlpha
			}
		}
		vp8x := []byte{flags, 0, 0, 0}
		vp8x = e.byteOrder.AppendUint32(vp8x, width-1)[:7]
		vp8x = e.byteOrder.AppendUint32(vp8x, height-1)[:10]
		vp8xChunk := appendWebPChunk(nil, webpFCC.vp8x, vp8x)
		newRiffSize := uint64(4+len(vp8xChunk)) + uint64(len(e.b)-12) + uint64(len(pad)) + uint64(len(tail))
		if newRiffSize > math.MaxUint32 {
			return errors.New("webp: file too large")
		}
		e.byteOrder.PutUint32(riffHdr[4:], uint32(newRiffSize))
		return e.writeChunks(riffHdr[:], vp8xChunk, e.b[12:], pad, tail)
	default:
		return newInvalidFormatErrorf("webp: unexpected first chunk %q", firstFCC)
	}
}

func appendWebPChunk(buf []byte, fcc fourCC, data []byte) []byte {
	buf = append(buf, fcc[:]...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(data)))
	buf = append(buf, data...)
	if len(data)%2 == 1 {
		buf = append(buf, 0)
	}
	return buf
}
