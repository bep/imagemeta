// Copyright 2026 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type imageEncoderJPEG struct {
	*baseEncoder
}

// encode inserts the EXIF and XMP APP1 segments after SOI and any APP0 (JFIF) segments.
func (e *imageEncoderJPEG) encode() error {
	if soi := e.read2(); e.err != nil || soi != jpegMarker.soi {
		return newInvalidFormatErrorf("jpeg: missing SOI marker")
	}

	insertAt := e.pos
	inLeadingAPP0 := true
	seenEnd := false

	for e.remaining() >= 2 {
		segStart := e.pos
		if e.read1() != 0xFF {
			return newInvalidFormatErrorf("jpeg: expected marker at offset %d", segStart)
		}
		var m byte
		for {
			if e.remaining() == 0 {
				return newInvalidFormatErrorf("jpeg: truncated marker at offset %d", segStart)
			}
			m = e.read1()
			if m != 0xFF {
				break
			}
		}
		if m == 0x00 {
			return newInvalidFormatErrorf("jpeg: invalid marker 0x00 at offset %d", segStart)
		}
		marker := 0xFF00 | uint16(m)
		if marker == jpegMarker.sos || marker == jpegMarker.eoi {
			// SOS or EOI.
			seenEnd = true
			break
		}
		if m == 0x01 || (m >= 0xD0 && m <= 0xD7) {
			inLeadingAPP0 = false
			continue
		}
		length := int(e.read2())
		if e.err != nil || length < 2 || length-2 > e.remaining() {
			return newInvalidFormatErrorf("jpeg: invalid segment length at offset %d", segStart)
		}
		seg := e.readBytes(length - 2)
		if marker == jpegMarker.app1EXIF && (bytes.HasPrefix(seg, markerEXIF) || bytes.HasPrefix(seg, markerXMP)) {
			return errMetadataExistsf("jpeg: APP1 segment at offset %d", segStart)
		}
		if marker == jpegMarker.app0 && inLeadingAPP0 {
			insertAt = e.pos
		} else {
			inLeadingAPP0 = false
		}
	}
	if !seenEnd {
		return newInvalidFormatErrorf("jpeg: missing SOS marker")
	}

	var segs []byte
	var err error
	if exif := e.meta.encodeEXIF(); exif != nil {
		if segs, err = appendJPEGAPP1(segs, markerEXIFHeader, exif); err != nil {
			return err
		}
	}
	if segs, err = appendJPEGAPP1(segs, markerXMP, e.meta.encodeXMP()); err != nil {
		return err
	}

	return e.writeChunks(e.b[:insertAt], segs, e.b[insertAt:])
}

func appendJPEGAPP1(buf, header, data []byte) ([]byte, error) {
	n := 2 + len(header) + len(data)
	if n > 0xFFFF {
		return nil, fmt.Errorf("jpeg: metadata too large: %d bytes", n)
	}
	buf = binary.BigEndian.AppendUint16(buf, jpegMarker.app1EXIF)
	buf = binary.BigEndian.AppendUint16(buf, uint16(n))
	buf = append(buf, header...)
	return append(buf, data...), nil
}
