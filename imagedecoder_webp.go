package imagemeta

import (
	"errors"
	"fmt"
	"strings"
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
	return "invalid format: " + e.Err.Error()
}

// Is reports whether the target error is an InvalidFormatError.
func (e *InvalidFormatError) Is(target error) bool {
	_, ok := target.(*InvalidFormatError)
	return ok
}

func newInvalidFormatErrorf(format string, args ...any) error {
	return &InvalidFormatError{fmt.Errorf(format, args...)}
}

func newInvalidFormatError(err error) error {
	return &InvalidFormatError{err}
}

// These error situations comes from the Go Fuzz modifying the input data to trigger panics.
// We want to separate panics that we can do something about and "invalid format" errors.
var invalidFormatErrorStrings = []string{
	"unexpected EOF",
}

func isInvalidFormatErrorCandidate(err error) bool {
	for _, s := range invalidFormatErrorStrings {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
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
