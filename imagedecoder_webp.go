package imagemeta

import (
	"fmt"
	"io"
)

var (
	fccRIFF = fourCC{'R', 'I', 'F', 'F'}
	fccWEBP = fourCC{'W', 'E', 'B', 'P'}
	fccVP8X = fourCC{'V', 'P', '8', 'X'}
	fccEXIF = fourCC{'E', 'X', 'I', 'F'}
	fccXMP  = fourCC{'X', 'M', 'P', ' '}
)

// ErrInvalidFormat is returned when the format is not recognized.
var ErrInvalidFormat = fmt.Errorf("imagemeta: invalid format")

type baseStreamingDecoder struct {
	*streamReader
	opts Options
	err  error
}

func (d *baseStreamingDecoder) streamErr() error {
	if d.err != nil {
		return d.err
	}
	return d.readErr
}

type decoderWebP struct {
	*baseStreamingDecoder
}

func (e *decoderWebP) decode() error {

	// These are the sources we currently support in WebP.
	sourceSet := TagSourceEXIF | TagSourceXMP
	// Remove sources that are not requested.
	sourceSet = sourceSet & e.opts.Sources

	if sourceSet.IsZero() {
		// Done.
		return nil
	}

	var (
		buf [10]byte
	)

	var chunkID fourCC
	// Read the RIFF header.
	e.readBytes(chunkID[:])
	if chunkID != fccRIFF {
		return ErrInvalidFormat
	}

	// File size.
	e.skip(4)

	e.readBytes(chunkID[:])
	if chunkID != fccWEBP {
		return ErrInvalidFormat
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

		switch chunkID {

		case fccVP8X:
			if chunkLen != 10 {
				return ErrInvalidFormat
			}

			const (
				xmpMetadataBit  = 1 << 2
				exifMetadataBit = 1 << 3
			)

			e.readBytes(buf[:])

			hasEXIF := buf[0]&exifMetadataBit != 0
			hasXMP := buf[0]&xmpMetadataBit != 0

			if !hasEXIF {
				sourceSet = sourceSet.Remove(TagSourceEXIF)
			}
			if !hasXMP {
				sourceSet = sourceSet.Remove(TagSourceXMP)
			}

			if !hasEXIF && !hasXMP {
				return nil
			}
		case fccEXIF:
			if !sourceSet.Has(TagSourceEXIF) {
				continue
			}
			r := io.LimitReader(e.r, int64(chunkLen))
			dec := newMetaDecoderEXIF(r, e.opts.HandleTag)

			if err := dec.decode(); err != nil {
				return err
			}
			sourceSet = sourceSet.Remove(TagSourceEXIF)
		case fccXMP:
			if !sourceSet.Has(TagSourceXMP) {
				continue
			}
			sourceSet = sourceSet.Remove(TagSourceXMP)
			r := io.LimitReader(e.r, int64(chunkLen))
			if err := decodeXMP(r, e.opts.HandleTag); err != nil {
				return err
			}
		default:
			e.skip(int64(chunkLen))
		}
	}
}
