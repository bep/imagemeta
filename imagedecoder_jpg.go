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

	if soi != jpegMarker.soi {
		return nil
	}

	// These are the sources we support.
	sourceSet := EXIF | IPTC | XMP | CONFIG
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

		if marker == jpegMarker.sos {
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

		if marker == jpegMarker.app1EXIF && sourceSet.Has(EXIF) {
			sourceSet = sourceSet.Remove(EXIF)
			if err := e.handleEXIF(int64(length)); err != nil {
				return err
			}
			continue
		}

		if marker == jpegMarker.app13 && sourceSet.Has(IPTC) {
			sourceSet = sourceSet.Remove(IPTC)
			if err := e.handleIPTC(int(length)); err != nil {
				return err
			}
			continue
		}

		if marker == jpegMarker.app1XMP && sourceSet.Has(XMP) {
			const xmpMarkerLen = 29
			oldPos := e.pos()
			b, err := e.readBytesVolatileE(xmpMarkerLen)

			if err != nil && err != io.ErrUnexpectedEOF {
				return err
			}

			if err == nil && bytes.Equal(b, markerXMP) {
				length -= xmpMarkerLen
				sourceSet = sourceSet.Remove(XMP)
				// Use bufferedReader so the entire XMP segment is consumed
				// from e.r up front. Otherwise xml.Decoder's internal
				// bufio.Reader pre-fetches bytes past where XML parsing
				// stops (XMP packets typically have ~2 KB of trailing
				// padding after </x:xmpmeta>), leaving e.r misaligned for
				// the next JPEG segment marker.
				r, berr := e.bufferedReader(int64(length))
				if berr != nil {
					return berr
				}
				if err := decodeXMP(r, e.opts); err != nil {
					r.Close()
					return err
				}
				r.Close()
				continue
			} else {
				// Not XMP, rewind.
				e.seek(oldPos)
			}

		}

		// SOF markers contain image dimensions.
		if sourceSet.Has(CONFIG) && (marker == jpegMarker.sof0 || marker == jpegMarker.sof1 || marker == jpegMarker.sof2) {
			sourceSet = sourceSet.Remove(CONFIG)
			e.skip(1) // Skip precision byte.
			height := int(e.read2())
			width := int(e.read2())
			e.result.ImageConfig = ImageConfig{
				Width:  width,
				Height: height,
			}
			e.skip(int64(length) - 5) // Skip remaining bytes (precision + height + width = 5).
			continue
		}

		e.skip(int64(length))
	}
}

func (e *imageDecoderJPEG) handleEXIF(length int64) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// Recover from panic in EXIF decoder (e.g., errStop).
			// This allows the JPEG decoder to continue processing other sources.
			if rerr, ok := r.(error); ok && rerr != errStop {
				err = rerr
			}
		}
	}()

	thumbnailOffset := e.pos()
	r, err := e.bufferedReader(length)
	if err != nil {
		return err
	}
	defer r.Close()
	exifr := newMetaDecoderEXIF(r, e.byteOrder, thumbnailOffset, e.opts)

	header := exifr.read4()
	if header != jpegMarker.exifHeader {
		return nil
	}
	exifr.skip(2)

	return exifr.decode()
}

func (e *imageDecoderJPEG) handleIPTC(length int) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if rerr, ok := r.(error); ok && rerr != errStop {
				err = rerr
			}
		}
	}()

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
