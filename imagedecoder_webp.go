// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import "io"

// WebP chunk types.
var webpFCC = struct {
	riff, webp      fourCC
	vp8x, vp8, vp8l fourCC
	exif, xmp, anmf fourCC
}{
	riff: fourCC{'R', 'I', 'F', 'F'},
	webp: fourCC{'W', 'E', 'B', 'P'},
	vp8x: fourCC{'V', 'P', '8', 'X'},
	vp8:  fourCC{'V', 'P', '8', ' '},
	vp8l: fourCC{'V', 'P', '8', 'L'},
	exif: fourCC{'E', 'X', 'I', 'F'},
	xmp:  fourCC{'X', 'M', 'P', ' '},
	anmf: fourCC{'A', 'N', 'M', 'F'},
}

func (e *decoderWebP) decode() error {
	// These are the sources we currently support in WebP.
	sourceSet := EXIF | XMP | CONFIG
	// Remove sources that are not requested.
	sourceSet = sourceSet & e.opts.Sources

	if sourceSet.IsZero() {
		// Done.
		return nil
	}

	var buf [10]byte

	var chunkID fourCC
	// Read the RIFF header.
	e.readBytes(chunkID[:])
	if chunkID != webpFCC.riff {
		return errInvalidFormat
	}

	riffEnd := e.pos() + 4 + int64(e.read4())

	e.readBytes(chunkID[:])
	if chunkID != webpFCC.webp {
		return errInvalidFormat
	}

	var countFrames bool

	for {
		if sourceSet.IsZero() && !countFrames {
			return nil
		}

		if e.pos()+8 > riffEnd {
			return nil
		}

		e.readBytes(chunkID[:])
		if e.isEOF {
			return nil
		}

		chunkLen := e.read4()
		if e.isEOF || e.pos()+int64(chunkLen) > riffEnd {
			return nil
		}
		// Chunks are padded to an even size.
		next := e.pos() + int64(chunkLen) + int64(chunkLen&1)

		switch {
		case chunkID == webpFCC.vp8x:
			if chunkLen != 10 {
				return errInvalidFormat
			}

			const (
				animationBit    = 1 << 1
				xmpMetadataBit  = 1 << 2
				exifMetadataBit = 1 << 3
			)

			e.readBytes(buf[:])

			hasEXIF := buf[0]&exifMetadataBit != 0
			hasXMP := buf[0]&xmpMetadataBit != 0

			if !hasEXIF {
				sourceSet = sourceSet.Remove(EXIF)
			}
			if !hasXMP {
				sourceSet = sourceSet.Remove(XMP)
			}

			if sourceSet.Has(CONFIG) {
				// Bytes 4-6: canvas width minus 1 (24-bit little-endian)
				// Bytes 7-9: canvas height minus 1 (24-bit little-endian)
				width := int(buf[4]) | int(buf[5])<<8 | int(buf[6])<<16 + 1
				height := int(buf[7]) | int(buf[8])<<8 | int(buf[9])<<16 + 1
				e.result.ImageConfig = ImageConfig{
					Width:  width,
					Height: height,
				}
				sourceSet = sourceSet.Remove(CONFIG)
				countFrames = buf[0]&animationBit != 0
				if countFrames {
					// Don't count a truncated last frame.
					pos := e.pos()
					end, err := e.r.Seek(0, io.SeekEnd)
					if err != nil {
						return err
					}
					riffEnd = min(riffEnd, end)
					e.seek(pos)
				}
			}
		case chunkID == webpFCC.anmf && countFrames:
			e.result.ImageConfig.FrameCount++
		case chunkID == webpFCC.exif && sourceSet.Has(EXIF):
			sourceSet = sourceSet.Remove(EXIF)
			thumbnailOffset := e.pos()
			if err := func() error {
				r, err := e.bufferedReader(int64(chunkLen))
				if err != nil {
					return err
				}
				defer r.Close()
				dec := newMetaDecoderEXIF(r, e.byteOrder, thumbnailOffset, e.opts)
				return dec.decode()
			}(); err != nil {
				return err
			}

		case chunkID == webpFCC.xmp && sourceSet.Has(XMP):
			sourceSet = sourceSet.Remove(XMP)
			if err := func() error {
				r, err := e.bufferedReader(int64(chunkLen))
				if err != nil {
					return err
				}
				defer r.Close()
				return decodeXMP(r, e.opts)
			}(); err != nil {
				return err
			}

		case chunkID == webpFCC.vp8 && sourceSet.Has(CONFIG):
			sourceSet = sourceSet.Remove(CONFIG)
			// VP8 lossy format: read frame header for dimensions.
			e.readBytes(buf[:10])
			// Check for VP8 signature: 0x9D 0x01 0x2A at bytes 3-5.
			if buf[3] == 0x9D && buf[4] == 0x01 && buf[5] == 0x2A {
				// Width and height are 14-bit values in bytes 6-9.
				width := int(buf[6]) | int(buf[7]&0x3F)<<8
				height := int(buf[8]) | int(buf[9]&0x3F)<<8
				e.result.ImageConfig = ImageConfig{
					Width:  width,
					Height: height,
				}
			}

		case chunkID == webpFCC.vp8l && sourceSet.Has(CONFIG):
			sourceSet = sourceSet.Remove(CONFIG)
			// VP8L lossless format.
			e.readBytes(buf[:5])
			// Check for VP8L signature: 0x2F.
			if buf[0] == 0x2F {
				// Width and height are packed in bytes 1-4.
				// Bits 0-13: width - 1, bits 14-27: height - 1.
				bits := uint32(buf[1]) | uint32(buf[2])<<8 | uint32(buf[3])<<16 | uint32(buf[4])<<24
				width := int(bits&0x3FFF) + 1
				height := int((bits>>14)&0x3FFF) + 1
				e.result.ImageConfig = ImageConfig{
					Width:  width,
					Height: height,
				}
			}
		}

		e.seek(next)
	}
}
