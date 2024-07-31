// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

var (
	fccRIFF = fourCC{'R', 'I', 'F', 'F'}
	fccWEBP = fourCC{'W', 'E', 'B', 'P'}
	fccVP8X = fourCC{'V', 'P', '8', 'X'}
	fccEXIF = fourCC{'E', 'X', 'I', 'F'}
	fccXMP  = fourCC{'X', 'M', 'P', ' '}
)

func (e *decoderWebP) decode() error {
	// These are the sources we currently support in WebP.
	sourceSet := EXIF | XMP
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

		default:
			e.skip(int64(chunkLen))
		}
	}
}
