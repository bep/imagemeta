package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"path"
	"strings"
)

var markerXMP = []byte("http://ns.adobe.com/xap/1.0/\x00")

const (
	markerSOI             = 0xffd8
	markerApp1EXIF        = 0xffe1
	markerrApp1XMP        = 0xffe1
	markerApp13           = 0xffed
	markerSOS             = 0xffda
	exifPointer           = 0x8769
	exifHeader            = 0x45786966
	pngEXIFMarker         = 0x65584966
	byteOrderBigEndian    = 0x4d4d
	byteOrderLittleEndian = 0x4949

	tagNameThumbnailOffset = "ThumbnailOffset"
)

//go:generate stringer -type=exifType

const (
	exitTypeUnsignedByte  exifType = 1
	exitTypeUnsignedASCII exifType = 2
	exitTypeUnsignedShort exifType = 3
	exitTypeUnsignedLong  exifType = 4
	exitTypeUnsignedRat   exifType = 5
	exitTypeSignedByte    exifType = 6
	exitTypeUndef         exifType = 7
	exitTypeSignedShort   exifType = 8
	exitTypeSignedLong    exifType = 9
	exitTypeSignedRat     exifType = 10
	exitTypeSignedFloat   exifType = 11
	exitTypeSignedDouble  exifType = 12
)

// Size in bytes of each type.
var exifTypeSize = map[exifType]uint32{
	exitTypeUnsignedByte:  1,
	exitTypeUnsignedASCII: 1,
	exitTypeUnsignedShort: 2,
	exitTypeUnsignedLong:  4,
	exitTypeUnsignedRat:   8,
	exitTypeSignedByte:    1,
	exitTypeUndef:         1,
	exitTypeSignedShort:   2,
	exitTypeSignedLong:    4,
	exitTypeSignedRat:     8,
	exitTypeSignedFloat:   4,
	exitTypeSignedDouble:  8,
}

var (
	exifFieldsAll   = map[uint16]string{}
	exifIFDPointers = map[uint16]string{
		0x8769: "ExifIFDP",
		0x8825: "GPSInfoIFD",
		0xa005: "InteroperabilityIFD",
	}
)

var (
	exifConverters        = &vc{}
	exifValueConverterMap = map[string]valueConverter{
		"ApertureValue":           exifConverters.convertAPEXToFNumber,
		"MaxApertureValue":        exifConverters.convertAPEXToFNumber,
		"ShutterSpeedValue":       exifConverters.convertAPEXToSeconds,
		"GPSLatitude":             exifConverters.convertDegreesToDecimal,
		"GPSLongitude":            exifConverters.convertDegreesToDecimal,
		"GPSMeasureMode":          exifConverters.convertStringToInt,
		"SubSecTimeDigitized":     exifConverters.convertStringToInt,
		"SubSecTimeOriginal":      exifConverters.convertStringToInt,
		"SubSecTime":              exifConverters.convertStringToInt,
		"GPSTimeStamp":            exifConverters.convertToTimestampString,
		"GPSVersionID":            exifConverters.convertBytesToStringSpaceDelim,
		"SubjectArea":             exifConverters.convertNumbersToSpaceLimited,
		"ComponentsConfiguration": exifConverters.convertBytesToStringSpaceDelim,
		"LensInfo":                exifConverters.convertRatsToSpaceLimited,
		"UserComment": func(byteOrder binary.ByteOrder, v any) any {
			return strings.TrimPrefix(printableString(toString(v)), "ASCII")
		},
		"CFAPattern": func(byteOrder binary.ByteOrder, v any) any {
			b := v.([]byte)
			horizontalRepeat := byteOrder.Uint16(b[:2])
			verticalRepeat := byteOrder.Uint16(b[2:])
			len := int(horizontalRepeat) * int(verticalRepeat)
			val := b[4 : 4+len]
			return fmt.Sprintf("%d %d %s", horizontalRepeat, verticalRepeat, exifConverters.convertBytesToStringSpaceDelim(byteOrder, val))
		},
	}
)

func newMetaDecoderEXIF(r io.Reader, thumbnailOffset int, opts Options) *metaDecoderEXIF {
	s := newStreamReader(r)
	return &metaDecoderEXIF{
		thumbnailOffset: thumbnailOffset,
		streamReader:    s,
		opts:            opts,
	}
}

// exifType represents the basic tiff tag data types.
type exifType uint16

type metaDecoderEXIF struct {
	*streamReader
	thumbnailOffset int

	opts Options
}

func (e *metaDecoderEXIF) convertValue(typ exifType, r io.Reader) any {
	switch typ {
	case exitTypeUnsignedByte, exitTypeUndef:
		return e.read1r(r)
	case exitTypeUnsignedShort:
		return e.read2r(r)
	case exitTypeUnsignedLong:
		return e.read4r(r)
	case exitTypeSignedLong:
		return e.read4sr(r)
	case exitTypeUnsignedRat:
		n, d := e.read4r(r), e.read4r(r)
		if d == 0 {
			return math.Inf(1)
		}
		return NewRat[uint32](n, d)
	case exitTypeSignedRat:
		n, d := e.read4sr(r), e.read4sr(r)
		return NewRat[int32](n, d)
	default:
		// TODO1
		panic(fmt.Errorf("exif type %d not implemented", typ))
	}
}

func (e *metaDecoderEXIF) convertValues(typ exifType, count, len int, r io.Reader) any {
	if count == 0 {
		return nil
	}

	if typ == exitTypeUnsignedASCII {
		b := e.readBytesFromRVolatile(len, r)
		return string(trimBytesNulls(b[:count]))
	}

	if count == 1 {
		return e.convertValue(typ, r)
	}

	values := make([]any, count)
	allBytes := true
	for i := 0; i < count; i++ {
		v := e.convertValue(typ, r)
		values[i] = v
		if allBytes {
			_, ok := v.(byte)
			if !ok {
				allBytes = false
			}
		}
	}

	if allBytes {
		bs := make([]byte, count)
		for i, v := range values {
			b := v.(byte)
			bs[i] = b
		}
		return bs

	}
	return values
}

func (e *metaDecoderEXIF) decode() (err error) {
	e.readerOffset = e.pos()
	byteOrderTag := e.read2()

	switch byteOrderTag {
	case byteOrderBigEndian:
		e.byteOrder = binary.BigEndian
	case byteOrderLittleEndian:
		e.byteOrder = binary.LittleEndian
	default:
		return nil
	}

	e.skip(2)

	// Main image.
	ifd0Offset := e.read4()

	if ifd0Offset < 8 {
		return nil
	}

	e.skip(int64(ifd0Offset - 8))

	if err := e.decodeTags("IFD0"); err != nil {
		return err
	}

	// Thumbnail IFD.
	ifd1Offset := e.read4()
	if ifd1Offset == 0 {
		// No more.
		return nil
	}
	e.seek(int(ifd1Offset) + e.readerOffset)

	if err := e.decodeTags("IFD1"); err != nil {
		return err
	}

	return nil
}

// A tag is represented in 12 bytes:
//   - 2 bytes for the tag ID
//   - 2 bytes for the data type
//   - 4 bytes for the number of data values of the specified type
//   - 4 bytes for the value itself, if it fits, otherwise for a pointer to another location where the data may be found;
//     this could be a pointer to the beginning of another IFD.
func (e *metaDecoderEXIF) decodeTag(namespace string) error {
	tagID := e.read2()

	tagName := exifFieldsAll[tagID]
	if tagName == "" {
		tagName = fmt.Sprintf("%s0x%x", UnknownPrefix, tagID)
	}

	if strings.Contains(tagName, " ") {
		// Space separated, pick first.
		parts := strings.Split(tagName, " ")
		tagName = parts[0]

	}

	dataType := e.read2()
	count := e.read4()
	if count > 0x10000 {
		e.skip(4)
		return nil
	}

	ifd, isIFDPointer := exifIFDPointers[tagID]

	tagInfo := TagInfo{
		Source:    EXIF,
		Tag:       tagName,
		Namespace: namespace,
	}

	if !isIFDPointer && !e.opts.ShouldHandleTag(tagInfo) {
		e.skip(4)
		return nil
	}

	typ := exifType(dataType)

	size, ok := exifTypeSize[typ]
	if !ok {
		return fmt.Errorf("%w: unknown EXIF type %d", errInvalidFormat, typ)
	}
	valLen := size * count

	var r io.Reader = e.r

	if valLen > 4 {
		valueOffset := e.read4()
		offset := valueOffset + uint32(e.readerOffset)
		r = io.NewSectionReader(e.r, int64(offset), int64(valLen))
	}

	val := e.convertValues(typ, int(count), int(valLen), r)
	if valLen <= 4 {
		padding := 4 - valLen
		if padding > 0 {
			e.skip(int64(padding))
		}
	}

	if isIFDPointer {
		offset := val.(uint32)
		namespace := path.Join(namespace, ifd)
		return e.decodeTagsAt(namespace, int(offset))
	}

	if convert, found := exifValueConverterMap[tagName]; found {
		val = convert(e.byteOrder, val)
	} else {
		val = toPrintableValue(val)
	}

	if val == nil {
		val = ""
	}

	if tagName == tagNameThumbnailOffset {
		// When set, thumbnailOffset is set to the offset of the EXIF data in the original file.
		val = val.(uint32) + uint32(e.readerOffset+e.thumbnailOffset)
	}

	tagInfo.Value = val

	if err := e.opts.HandleTag(tagInfo); err != nil {
		return err
	}

	return nil
}

func (e *metaDecoderEXIF) decodeTags(namespace string) error {
	numTags := e.read2()

	for i := 0; i < int(numTags); i++ {
		if err := e.decodeTag(namespace); err != nil {
			return err
		}
	}

	return nil
}

func (e *metaDecoderEXIF) decodeTagsAt(namespace string, offset int) error {
	oldPos := e.pos()
	defer func() {
		e.seek(oldPos)
	}()
	e.seek(offset + e.readerOffset)
	return e.decodeTags(namespace)
}

type valueConverter func(binary.ByteOrder, any) any

func init() {
	for k, v := range exifFields {
		exifFieldsAll[k] = v
	}
	for k, v := range exifFieldsGPS {
		exifFieldsAll[k] = v
	}
}
