package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

type imageDecoderJPEG struct {
	*baseStreamingDecoder
}

func (e *imageDecoderJPEG) decode() (err error) {
	// JPEG SOI marker.
	var soi uint16
	if err = e.readFullE(&soi); err != nil {
		return nil
	}
	if soi != markerSOI {
		return
	}

	findMarker := func(markerToFind uint16) int {
		for {
			var marker, length uint16
			if err = e.readFullE(&marker); err != nil {
				return -1
			}
			if err = e.readFullE(&length); err != nil {
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

	if e.opts.SourceSet[TagSourceEXIF] {
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

	if e.opts.SourceSet[TagSourceIPTC] {
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

// exifTy

// exifType represents the basic tiff tag data types.
type exifType uint16

// A tag is represented in 12 bytes:
//   - 2 bytes for the tag ID
//   - 2 bytes for the data type
//   - 4 bytes for the number of data values of the specified type
//   - 4 bytes for the value itself, if it fits, otherwise for a pointer to another location where the data may be found;
//     this could be a pointer to the beginning of another IFD
func (e *metaDecoderEXIF) decodeTag() error {
	tagID := e.read2()
	tagName := fieldsAll[tagID]
	if tagName == "" {
		tagName = fmt.Sprintf("%s0x%x", UnknownPrefix, tagID)
	}

	dataType := e.read2()
	count := e.read4()
	if count > 0x10000 {
		e.skip(4)
		return nil
	}
	if count == 0 {
		count = 1 // TODO1 make this 0.
	}
	typ := exifType(dataType)

	size, ok := typeSize[typ]
	if !ok {
		return fmt.Errorf("unknown type for tag %s %d", tagName, typ)
	}
	valLen := size * count

	var r io.Reader = e.r

	if valLen > 4 {
		offset := e.read4() + uint32(e.readerOffset)
		r = io.NewSectionReader(e.r, int64(offset), int64(valLen))
	}

	val := e.convertValues(typ, int(count), int(valLen), r)

	if valLen <= 4 {
		padding := 4 - valLen
		if padding > 0 {
			e.skip(int64(padding))
		}
	}

	if strings.HasSuffix(tagName, "IFDPointer") {
		offset := val.(uint32)
		return e.decodeTagsAT(int(offset))
	}

	tagInfo := TagInfo{
		Source: TagSourceEXIF,
		Tag:    tagName,
		Value:  val,
	}

	if err := e.handleTag(tagInfo); err != nil {
		return err
	}

	return nil
}
