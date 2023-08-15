package imagemeta

import (
	"encoding/binary"
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
	sourceSet := TagSourceEXIF | TagSourceIPTC | TagSourceXMP
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
			return ErrInvalidFormat
		}
		length -= 2

		if marker == markerApp1EXIF && sourceSet.Has(TagSourceEXIF) {
			sourceSet = sourceSet.Remove(TagSourceEXIF)
			if err := e.handleEXIF(int(length)); err != nil {
				return err
			}
			continue
		}

		if marker == markerApp13 && sourceSet.Has(TagSourceIPTC) {
			sourceSet = sourceSet.Remove(TagSourceIPTC)
			if err := e.handleIPTC(int(length)); err != nil {
				return err
			}
			continue
		}

		if marker == markerrApp1XMP && sourceSet.Has(TagSourceXMP) {
			sourceSet = sourceSet.Remove(TagSourceXMP)
			const xmpIDLen = 29
			if length < xmpIDLen {
				return ErrInvalidFormat
			}
			e.skip(int64(xmpIDLen))
			length -= xmpIDLen
			r := io.LimitReader(e.r, int64(length))
			if err := decodeXMP(r, e.opts); err != nil {
				return err
			}
			continue
		}

		e.skip(int64(length))
	}
}

func (e *imageDecoderJPEG) handleIPTC(length int) error {
	// EXIF may be stored in a different order, but IPTC is always big-endian.
	e.byteOrder = binary.BigEndian
	r, err := e.bufferedReader(length)
	if err != nil {
		return err
	}
	defer r.Close()
	dec := newMetaDecoderIPTC(r, e.opts.HandleTag)
	return dec.decode()
}

func (e *imageDecoderJPEG) handleEXIF(length int) error {
	r, err := e.bufferedReader(length)
	if err != nil {
		return err
	}
	defer r.Close()
	exifr := newMetaDecoderEXIF(r, e.opts.HandleTag)

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
