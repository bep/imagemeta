// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

var (
	fccRIFF = fourCC{'R', 'I', 'F', 'F'}
	fccWEBP = fourCC{'W', 'E', 'B', 'P'}
	fccVP8X = fourCC{'V', 'P', '8', 'X'}
	fccVP8  = fourCC{'V', 'P', '8', ' '}
	fccVP8L = fourCC{'V', 'P', '8', 'L'}
	fccEXIF = fourCC{'E', 'X', 'I', 'F'}
	fccXMP  = fourCC{'X', 'M', 'P', ' '}
)

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
	if chunkID != fccRIFF {
		return errInvalidFormat
	}

	// File size.
	e.skip(4)

	e.readBytes(chunkID[:])
	if chunkID != fccWEBP {
		return errInvalidFormat
	}

	for {
		if sourceSet.IsZero() {
			return nil
		}

		e.readBytes(chunkID[:])
		if e.isEOF {
			return nil
		}

		chunkLen := e.read4()

		switch {
		case chunkID == fccVP8X:
			if chunkLen != 10 {
				return errInvalidFormat
			}

			const (
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
			}

			if sourceSet.IsZero() {
				return nil
			}
		case chunkID == fccEXIF && sourceSet.Has(EXIF):
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

		case chunkID == fccXMP && sourceSet.Has(XMP):
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

		case chunkID == fccVP8 && sourceSet.Has(CONFIG):
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
			e.skip(int64(chunkLen) - 10)

		case chunkID == fccVP8L && sourceSet.Has(CONFIG):
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
			e.skip(int64(chunkLen) - 5)

		default:
			e.skip(int64(chunkLen))
		}
	}
}
