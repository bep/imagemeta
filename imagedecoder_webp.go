package imagemeta

import (
	"encoding/xml"
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

type decoderWebP struct {
	*baseStreamingDecoder
}

func (e *decoderWebP) decode() (err error) {
	handleEXIF := e.opts.Sources.Has(TagSourceEXIF)
	handleXMP := e.opts.Sources.Has(TagSourceXMP)

	if !handleEXIF && !handleXMP {
		return nil
	}

	var (
		buf [10]byte

		hasExif bool
		hasXMP  bool

		exifHandled = !handleEXIF
		xmpHandled  = !handleXMP
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
		if exifHandled && xmpHandled {
			return nil
		}

		e.readBytes(chunkID[:])

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

			hasExif = buf[0]&exifMetadataBit != 0
			hasXMP = buf[0]&xmpMetadataBit != 0

			if !hasExif && !hasXMP {
				return nil
			}
		case fccEXIF:
			if exifHandled {
				continue
			}
			r := io.LimitReader(e.r, int64(chunkLen))
			dec := newMetaDecoderEXIF(r, e.opts.HandleTag)

			if err := dec.decode(); err != nil {
				return err
			}

			exifHandled = true
		case fccXMP:
			if xmpHandled {
				continue
			}

			xmpHandled = true
			r := io.LimitReader(e.r, int64(chunkLen))
			var meta xmpmeta
			if err := xml.NewDecoder(r).Decode(&meta); err != nil {
				return err
			}

			for _, attr := range meta.RDF.Description.Attrs {
				tagInfo := TagInfo{
					Source:    TagSourceXMP,
					Tag:       attr.Name.Local,
					Namespace: attr.Name.Space,
					Value:     attr.Value,
				}

				if err := e.opts.HandleTag(tagInfo); err != nil {
					return err
				}
			}
		default:
			e.skip(int64(chunkLen))
		}
	}
}
