// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/binary"
)

type imageDecoderTIF struct {
	*baseStreamingDecoder
}

func (e *imageDecoderTIF) decode() error {
	const (
		meaningOfLife  = 42
		tagImageWidth  = 0x0100
		tagImageHeight = 0x0101
	)

	byteOrderTag := e.read2()
	switch byteOrderTag {
	case tiffMarker.byteOrderBE:
		e.byteOrder = binary.BigEndian
	case tiffMarker.byteOrderLE:
		e.byteOrder = binary.LittleEndian
	default:
		return errInvalidFormat
	}

	if id := e.read2(); id != meaningOfLife {
		return errInvalidFormat
	}

	ifdOffset := e.read4()

	if ifdOffset < 8 {
		return errInvalidFormat
	}

	e.skip(int64(ifdOffset - 8))

	// Handle CONFIG by scanning IFD0 for ImageWidth and ImageHeight.
	if e.opts.Sources.Has(CONFIG) {
		ifdPos := e.pos()
		numTags := e.read2()
		var width, height int
		for range int(numTags) {
			tagID := e.read2()
			dataType := e.read2()
			count := e.read4()
			if tagID == tagImageWidth || tagID == tagImageHeight {
				var value int
				// Read value based on type: SHORT (3) or LONG (4).
				if dataType == 3 { // SHORT
					value = int(e.read2())
					e.skip(2) // Padding.
				} else { // LONG
					value = int(e.read4())
				}
				if tagID == tagImageWidth {
					width = value
				} else {
					height = value
				}
				if width > 0 && height > 0 {
					break
				}
			} else {
				e.skip(4) // Skip value/offset.
			}
			_ = count // Count is always 1 for these tags.
		}
		e.result.ImageConfig = ImageConfig{
			Width:  width,
			Height: height,
		}
		// If only CONFIG was requested, we're done.
		if e.opts.Sources == CONFIG {
			return nil
		}
		// Seek back to IFD start for EXIF decoder.
		e.seek(ifdPos)
	}

	dec := newMetaDecoderEXIFFromStreamReader(e.streamReader, 0, e.opts)

	return dec.decodeTags("IFD0")
}
