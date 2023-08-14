package imagemeta

import (
	"encoding/binary"
)

type imageDecoderJPEG struct {
	*baseStreamingDecoder
}

func (e *imageDecoderJPEG) decode() (err error) {
	// JPEG SOI marker.
	var soi uint16
	if soi, err = e.read2E(); err != nil {
		return nil
	}
	if soi != markerSOI {
		return
	}

	findMarker := func(markerToFind uint16) int {
		for {
			var marker, length uint16
			if marker, err = e.read2E(); err != nil {
				return -1
			}
			if length, err = e.read2E(); err != nil {
				return -1
			}

			// All JPEG markers begin with 0xff.
			if marker>>8 != 0xff {
				return -1
			}

			if marker == markerToFind {
				return int(length)
			}

			if length < 2 {
				return -1
			}

			e.skip(int64(length - 2))
		}
	}

	if e.opts.Sources.Has(TagSourceEXIF) {
		pos := e.pos()
		if length := findMarker(markerAPP1); length > 0 {
			err := func() error {
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

			}()

			if err != nil {
				return err
			}

		}
		e.seek(pos)
	}

	if e.opts.Sources.Has(TagSourceIPTC) {
		// EXIF may be stored in a different order, but IPTC is always big-endian.
		e.byteOrder = binary.BigEndian
		if length := findMarker(markerApp13); length > 0 {
			if err := func() error {
				r, err := e.bufferedReader(length)
				if err != nil {
					return err
				}
				defer r.Close()
				dec := newMetaDecoderIPTC(r, e.opts.HandleTag)
				return dec.decode()
			}(); err != nil {
				return err
			}
		}
	}
	return nil
}
