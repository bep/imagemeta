package imagemeta

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"
)

var bufferPool = &sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

func newStreamReader(r io.Reader) *streamReader {
	var rr Reader
	var ok bool
	rr, ok = r.(Reader)
	if !ok {
		bb, err := io.ReadAll(r)
		if err != nil {
			panic(err)
		}
		rr = bytes.NewReader(bb)
	}

	return &streamReader{
		r:         rr,
		byteOrder: binary.BigEndian,
	}
}

type decoder interface {
	decode() error
}

// A tag is represented in 12 bytes:
//   - 2 bytes for the tag ID
//   - 2 bytes for the data type
//   - 4 bytes for the number of data values of the specified type
//   - 4 bytes for the value itself, if it fits, otherwise for a pointer to another location where the data may be found;
//     this could be a pointer to the beginning of another IFD
func (e *decoderExif) decodeEXIFTag() error {
	tagID := e.read2()
	tagName := fieldsAll[tagID]
	if tagName == "" {
		tagName = fmt.Sprintf("%s0x%x", UnknownPrefix, tagID)
	}

	if false { //} !e.shouldDecode(tagName) {
		e.skip(10)
		return nil
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
		return e.decodeEXIFTagsAt(int(offset))
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

type decoderJPEG struct {
	*baseStreamingDecoder
}

func (e *decoderJPEG) decode() (err error) {
	defer func() {
		if r := recover(); r != nil {
			if r != errStop {
				panic(r)
			}
			if err == nil {
				err = e.err
			}
		}
	}()

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
		exifr := &decoderExif{
			streamReader: e.streamReader,
			handleTag:    e.opts.HandleTag,
		}

		pos := e.pos()
		if findMarker(markerAPP1) > 0 {
			header := exifr.read4()
			if header != exifHeader {
				return err
			}
			exifr.skip(2)
			if err := exifr.decode(); err != nil {
				return err
			}
		}
		e.seek(pos)
	}

	if e.opts.SourceSet[TagSourceIPTC] {
		// EXIF may be stored in a different order, but IPTC is always big-endian.
		e.byteOrder = binary.BigEndian
		if length := findMarker(markerApp13); length > 0 {
			if err := e.decodeIPTC(length); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *decoderJPEG) decodeIPTC(length int) (err error) {
	// Skip the IPTC header.
	e.skip(14)

	const iptcMetaDataBlockID = 0x0404

	decodeBlock := func() error {
		blockType := make([]byte, 4)
		e.readFull(blockType)
		if string(blockType) != "8BIM" {
			return errStop
		}

		identifier := e.read2()
		isMeta := identifier == iptcMetaDataBlockID

		nameLength := e.read1()
		if nameLength == 0 {
			nameLength = 2
		} else if nameLength%2 == 1 {
			nameLength++
		}

		e.skip(int64(nameLength - 1))
		dataSize := e.read4()

		if !isMeta {
			e.skip(int64(dataSize))
			return nil
		}

		// TODO1 extended datasets.

		if dataSize%2 != 0 {
			defer func() {
				// Skip padding byte.
				e.skip(1)
			}()
		}

		r := io.LimitReader(e.r, int64(dataSize))

		for {
			var marker uint8
			if err := binary.Read(r, e.byteOrder, &marker); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			if marker != 0x1C {
				return errStop
			}

			var recordType, datasetNumber uint8
			var recordSize uint16
			if err := binary.Read(r, e.byteOrder, &recordType); err != nil {
				return err
			}
			if err := binary.Read(r, e.byteOrder, &datasetNumber); err != nil {
				return err
			}
			if err := binary.Read(r, e.byteOrder, &recordSize); err != nil {
				return err
			}

			recordBytes := make([]byte, recordSize)
			if err := binary.Read(r, e.byteOrder, recordBytes); err != nil {
				return err
			}

			// TODO1 get an up to date field map.
			// TODO1 handle unkonwn dataset numbers.
			recordDef, ok := iptcFieldMap[datasetNumber]
			if !ok {
				fmt.Println("unknown datasetNumber", datasetNumber)
				continue
			}

			var v any
			switch recordDef.format {
			case "string":
				v = string(recordBytes)
			case "B": // TODO1 check these
				v = recordBytes
			}

			if err := e.opts.HandleTag(TagInfo{
				Source: TagSourceIPTC,
				Tag:    recordDef.name,
				Value:  v,
			}); err != nil {
				return err
			}

		}
	}

	for {
		if err := decodeBlock(); err != nil {
			if err == errStop {
				break
			}
			return err
		}
	}

	return nil

}

// exifTy

// exifType represents the basic tiff tag data types.
type exifType uint16

type streamReader struct {
	r Reader

	byteOrder binary.ByteOrder

	readErr      error
	readerOffset int
}

func (e *streamReader) pos() int {
	n, _ := e.r.Seek(0, 1)
	return int(n)
}

func (e *streamReader) read1() uint8 {
	return e.read1r(e.r)
}

func (e *streamReader) read1r(r io.Reader) uint8 {
	var v uint8
	e.readFullr(&v, r)
	return v
}

func (e *streamReader) read2() uint16 {
	return e.read2r(e.r)
}

func (e *streamReader) read2r(r io.Reader) uint16 {
	var v uint16
	e.readFullr(&v, r)
	return v
}

func (e *streamReader) read4() uint32 {
	return e.read4r(e.r)
}

func (e *streamReader) read4Signedr(r io.Reader) int32 {
	var v int32
	e.readFullr(&v, r)
	return v
}

func (e *streamReader) read4r(r io.Reader) uint32 {
	var v uint32
	e.readFullr(&v, r)
	return v
}

func (e *streamReader) readFull(v any) {
	e.readFullr(v, e.r)
}

func (e *streamReader) readFullE(v any) error {
	return e.readFullrE(v, e.r)
}

func (e *streamReader) readFullr(v any, r io.Reader) {
	if err := binary.Read(r, e.byteOrder, v); err != nil {
		e.stop(err)
	}
}

func (e *streamReader) readFullrE(v any, r io.Reader) error {
	return binary.Read(r, e.byteOrder, v)
}

func (e *streamReader) seek(pos int) {
	_, err := e.r.Seek(int64(pos), io.SeekStart)
	if err != nil {
		e.stop(err)
	}
}

func (e *streamReader) skip(n int64) {
	e.r.Seek(n, io.SeekCurrent)
}

func (e *streamReader) stop(err error) {
	if err != nil {
		e.readErr = err
	}
	panic(errStop)
}

func getBuffer() (buf *bytes.Buffer) {
	return bufferPool.Get().(*bytes.Buffer)
}

func decode(opts Options) error {
	br := &streamReader{
		r:         opts.R,
		byteOrder: binary.BigEndian,
	}

	base := &baseStreamingDecoder{
		streamReader: br,
		opts:         opts,
	}

	var dec decoder

	switch opts.ImageFormat {
	case ImageFormatJPEG:
		dec = &decoderJPEG{baseStreamingDecoder: base}
	case ImageFormatWebP:
		dec = &decoderWebP{baseStreamingDecoder: base}
	default:
		return fmt.Errorf("unsupported image format")

	}

	err := dec.decode()
	if err == ErrStopWalking {
		return nil
	}
	return err
}

func putBuffer(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}
