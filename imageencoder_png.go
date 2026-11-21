// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"slices"
)

var pngXMPiTXtHead = append(slices.Clone(pngXMPKeyword), 0, 0, 0, 0) // Uncompressed, no language tag or translated keyword.

type imageEncoderPNG struct {
	*baseEncoder
}

// encode inserts an eXIf and an iTXt (XMP) chunk after the IHDR chunk.
func (e *imageEncoderPNG) encode() error {
	if sig := e.readBytes(len(pngSignature)); e.err != nil || !bytes.Equal(sig, pngSignature) {
		return newInvalidFormatErrorf("png: invalid signature")
	}

	ihdrEnd := 0
	seenIEND := false
	for e.remaining() >= 12 {
		chunkStart := e.pos
		chunkLength := uint64(e.read4())
		tagID := e.readBytes(4)
		if chunkLength > 0x7FFFFFFF || chunkLength > uint64(e.remaining()-4) {
			return newInvalidFormatErrorf("png: invalid chunk length at offset %d", chunkStart)
		}
		data := e.readBytes(int(chunkLength))
		e.skip(4) // Skip CRC.
		switch {
		case bytes.Equal(tagID, pngTagIDIHDR):
			ihdrEnd = e.pos
		case bytes.Equal(tagID, pngTagIDExif):
			return errMetadataExistsf("png: eXIf chunk at offset %d", chunkStart)
		case bytes.Equal(tagID, pngTagIDiTXt) && bytes.HasPrefix(data, pngXMPKeyword):
			return errMetadataExistsf("png: XMP chunk at offset %d", chunkStart)
		}
		if bytes.Equal(tagID, pngTagIDIEND) {
			seenIEND = true
			break
		}
	}
	if ihdrEnd == 0 {
		return newInvalidFormatErrorf("png: missing IHDR chunk")
	}
	if !seenIEND {
		return newInvalidFormatErrorf("png: missing IEND chunk")
	}

	var chunks []byte
	if exif := e.meta.encodeEXIF(); exif != nil {
		chunks = appendPNGChunk(chunks, pngTagIDExif, exif)
	}
	chunks = appendPNGChunk(chunks, pngTagIDiTXt, slices.Concat(pngXMPiTXtHead, e.meta.encodeXMP()))

	return e.writeChunks(e.b[:ihdrEnd], chunks, e.b[ihdrEnd:])
}

func appendPNGChunk(buf, typ, data []byte) []byte {
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(data)))
	start := len(buf)
	buf = append(buf, typ...)
	buf = append(buf, data...)
	return binary.BigEndian.AppendUint32(buf, crc32.ChecksumIEEE(buf[start:]))
}
