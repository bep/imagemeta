// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"bytes"
	"io"
)

type imageDecoderJPEG struct {
	*baseStreamingDecoder
}

func (e *imageDecoderJPEG) decode() error {
	// JPEG SOI marker.
	soi, err := e.read2E()
	if err != nil {
		return nil
	}

	if soi != markerSOI {
		return nil
	}

	// These are the sources we support.
	sourceSet := EXIF | IPTC | XMP
	// Remove sources that are not requested.
	sourceSet = sourceSet & e.opts.Sources

	for {
		if sourceSet.IsZero() {
			// Done.
			return nil
		}
		marker := e.read2()
		if e.isEOF {
			return nil
		}

		if marker == 0 {
			continue
		}

		if marker == markerSOS {
			// Start of scan. We're done.
			return nil
		}

		// Read the 16-bit length of the segment. The value includes the 2 bytes for the
		// length itself, so we subtract 2 to get the number of remaining bytes.
		length := e.read2()
		if length < 2 {
			return errInvalidFormat
		}
		length -= 2

		if marker == markerApp1EXIF && sourceSet.Has(EXIF) {
			sourceSet = sourceSet.Remove(EXIF)
			if err := e.handleEXIF(int64(length)); err != nil {
				return err
			}
			continue
		}

		if marker == markerApp13 && sourceSet.Has(IPTC) {
			sourceSet = sourceSet.Remove(IPTC)
			if err := e.handleIPTC(int(length)); err != nil {
				return err
			}
			continue
		}

		if marker == markerrApp1XMP && sourceSet.Has(XMP) {
			const xmpMarkerLen = 29
			oldPos := e.pos()
			b, err := e.readBytesVolatileE(xmpMarkerLen)

			if err != nil && err != io.ErrUnexpectedEOF {
				return err
			}

			if err == nil && bytes.Equal(b, markerXMP) {
				length -= xmpMarkerLen
				sourceSet = sourceSet.Remove(XMP)
				r := io.LimitReader(e.r, int64(length))
				if err := decodeXMP(r, e.opts); err != nil {
					return err
				}
				continue
			} else {
				// Not XMP, rewind.
				e.seek(oldPos)
			}

		}

		e.skip(int64(length))
	}
}

func (e *imageDecoderJPEG) handleEXIF(length int64) error {
	thumbnailOffset := e.pos()
	r, err := e.bufferedReader(length)
	if err != nil {
		return err
	}
	defer r.Close()
	exifr := newMetaDecoderEXIF(r, e.byteOrder, thumbnailOffset, e.opts)

	header := exifr.read4()
	if header != exifHeader {
		return err
	}
	exifr.skip(2)

	if err := exifr.decode(); err != nil {
		return err
	}
	return nil
}

func (e *imageDecoderJPEG) handleIPTC(length int) error {
	const headerLength = 14
	// Skip the IPTC header.
	e.skip(headerLength)
	r, err := e.bufferedReader(int64(length - headerLength))
	if err != nil {
		return err
	}
	defer r.Close()
	dec := newMetaDecoderIPTC(r, e.opts)
	return dec.decodeBlocks()
}
