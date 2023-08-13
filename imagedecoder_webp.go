package imagemeta

import (
	"encoding/xml"
	"fmt"
	"io"

	"golang.org/x/image/riff"
)

var (
	fccVP8X = riff.FourCC{'V', 'P', '8', 'X'}
	fccWEBP = riff.FourCC{'W', 'E', 'B', 'P'}
	fccEXIF = riff.FourCC{'E', 'X', 'I', 'F'}
	fccXMP  = riff.FourCC{'X', 'M', 'P', ' '}
)

var errInvalidFormat = fmt.Errorf("imagemeta: invalid format")

type baseStreamingDecoder struct {
	*streamReader
	opts Options
	err  error
}

type decoderWebP struct {
	*baseStreamingDecoder
}

func (e *decoderWebP) decode() (err error) {
	handleEXIF := e.opts.SourceSet[TagSourceEXIF]
	handleXMP := e.opts.SourceSet[TagSourceXMP]

	if !handleEXIF && !handleXMP {
		return nil
	}

	r := e.streamReader.r

	formType, riffReader, err := riff.NewReader(r)
	if err != nil {
		return err
	}
	if formType != fccWEBP {
		return fmt.Errorf("imagemeta: not a WebP file")
	}

	var (
		buf [10]byte

		hasExif bool
		hasXMP  bool

		exifHandled = !handleEXIF
		xmpHandled  = !handleXMP
	)

	for {
		if exifHandled && xmpHandled {
			return nil
		}

		chunkID, chunkLen, chunkData, err := riffReader.Next()
		if err == io.EOF {
			err = errInvalidFormat
		}
		if err != nil {
			return err
		}

		switch chunkID {
		case fccVP8X:
			if chunkLen != 10 {
				return errInvalidFormat
			}
			const (
				xmpMetadataBit  = 1 << 2
				exifMetadataBit = 1 << 3
			)

			if _, err := io.ReadFull(chunkData, buf[:10]); err != nil {
				return err
			}

			hasExif = buf[0]&exifMetadataBit != 0
			hasXMP = buf[0]&xmpMetadataBit != 0

			if !hasExif && !hasXMP {
				return nil
			}

		case fccEXIF:
			if exifHandled {
				continue
			}
			dec := newMetaDecoderEXIF(chunkData, e.opts.HandleTag)

			if err := dec.decode(); err != nil {
				return err
			}

			exifHandled = true
		case fccXMP:
			if xmpHandled {
				continue
			}

			xmpHandled = true

			var meta xmpmeta
			if err := xml.NewDecoder(chunkData).Decode(&meta); err != nil {
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

		}

	}

}
