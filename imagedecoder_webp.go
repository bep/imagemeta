package imagemeta

import (
	"errors"
	"io"
)

var (
	fccRIFF = fourCC{'R', 'I', 'F', 'F'}
	fccWEBP = fourCC{'W', 'E', 'B', 'P'}
	fccVP8X = fourCC{'V', 'P', '8', 'X'}
	fccEXIF = fourCC{'E', 'X', 'I', 'F'}
	fccXMP  = fourCC{'X', 'M', 'P', ' '}
)

// errInvalidFormat is used when the format is invalid.
var errInvalidFormat = &InvalidFormatError{errors.New("invalid format")}

// IsInvalidFormat reports whether the error was an InvalidFormatError.
func IsInvalidFormat(err error) bool {
	return errors.Is(err, errInvalidFormat)
}

// InvalidFormatError is used when the format is invalid.
type InvalidFormatError struct {
	Err error
}

func (e *InvalidFormatError) Error() string {
	return e.Err.Error()
}

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

			if !hasEXIF && !hasXMP {
				return nil
			}
		case chunkID == fccEXIF && sourceSet.Has(EXIF):
			r := io.LimitReader(e.r, int64(chunkLen))
			dec := newMetaDecoderEXIF(r, e.opts.HandleTag)
			if err := dec.decode(); err != nil {
				return err
			}
			sourceSet = sourceSet.Remove(EXIF)
		case chunkID == fccXMP && sourceSet.Has(XMP):
			sourceSet = sourceSet.Remove(XMP)
			r := io.LimitReader(e.r, int64(chunkLen))
			if err := decodeXMP(r, e.opts); err != nil {
				return err
			}
		default:
			e.skip(int64(chunkLen))
		}
	}
}
