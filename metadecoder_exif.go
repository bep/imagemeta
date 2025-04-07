// Copyright 2024 Bjørn Erik Pedersen
// SPDX-License-Identifier: MIT

package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"maps"
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
	exifHeader            = 0x45786966
	byteOrderBigEndian    = 0x4d4d
	byteOrderLittleEndian = 0x4949

	tagNameThumbnailOffset = "ThumbnailOffset"
)

const (
	xmpMarker  = 0x02bc // EXIF ApplicationNotes
	iptcMarker = 0x83bb // EXIF IPTC-NAA
)

//go:generate stringer -type=exifType

const (
	exifTypeUnsignedByte1  exifType = 1
	exifTypeASCIIString1   exifType = 2
	exifTypeUnsignedShort2 exifType = 3
	exifTypeUnsignedLong4  exifType = 4
	exifTypeUnsignedRat8   exifType = 5
	exifTypeSignedByte1    exifType = 6
	exifTypeUndef1         exifType = 7
	exifTypeSignedShort2   exifType = 8
	exifTypeSignedLong4    exifType = 9
	exifTypeSignedRat8     exifType = 10
	exifTypeSignedFloat4   exifType = 11
	exifTypeSignedDouble8  exifType = 12
)

// Used for +inf/-inf/nan. This is in line with Exiftool.
const undef = "undef"

// Size in bytes of each type.
var exifTypeSize = map[exifType]uint32{
	exifTypeUnsignedByte1:  1,
	exifTypeASCIIString1:   1,
	exifTypeUnsignedShort2: 2,
	exifTypeUnsignedLong4:  4,
	exifTypeUnsignedRat8:   8,
	exifTypeSignedByte1:    1,
	exifTypeUndef1:         1,
	exifTypeSignedShort2:   2,
	exifTypeSignedLong4:    4,
	exifTypeSignedRat8:     8,
	exifTypeSignedFloat4:   4,
	exifTypeSignedDouble8:  8,
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
		"GPSSatellites":           exifConverters.convertStringToInt,
		"GPSTimeStamp":            exifConverters.convertToTimestampString,
		"GPSVersionID":            exifConverters.convertBytesToStringSpaceDelim,
		"SubjectArea":             exifConverters.convertNumbersToSpaceLimited,
		"BitsPerSample":           exifConverters.convertNumbersToSpaceLimited,
		"PageNumber":              exifConverters.convertNumbersToSpaceLimited,
		"StripByteCounts":         exifConverters.convertNumbersToSpaceLimited,
		"StripOffsets":            exifConverters.convertNumbersToSpaceLimited,
		"PrimaryChromaticities":   exifConverters.convertRatsToSpaceLimited,
		"WhitePoint":              exifConverters.convertRatsToSpaceLimited,
		"ReferenceBlackWhite":     exifConverters.convertRatsToSpaceLimited,
		"YCbCrCoefficients":       exifConverters.convertRatsToSpaceLimited,
		"ComponentsConfiguration": exifConverters.convertBytesToStringSpaceDelim,
		"LensInfo":                exifConverters.convertRatsToSpaceLimited,
		"Padding":                 exifConverters.convertBinaryData,
		"UserComment":             exifConverters.convertUserComment,
		"CFAPattern": func(ctx valueConverterContext, v any) any {
			b := v.([]byte)
			horizontalRepeat := ctx.s.byteOrder.Uint16(b[:2])
			verticalRepeat := ctx.s.byteOrder.Uint16(b[2:])
			repeatLen := int(horizontalRepeat) * int(verticalRepeat)
			hi := 4 + repeatLen
			if hi > len(b) {
				// See issue 34.
				// There are cameras that writes CFAPattern with a byte order that's not the one specified in the EXIF header.
				order := ctx.s.otherByteOrder()
				horizontalRepeat = order.Uint16(b[:2])
				verticalRepeat = order.Uint16(b[2:])
				repeatLen := int(horizontalRepeat) * int(verticalRepeat)
				hi = 4 + repeatLen
				if hi > len(b) {
					// Just return the raw bytes.
					return trimBytesNulls(b)
				}
			}

			val := b[4:hi]
			return fmt.Sprintf("%d %d %s", horizontalRepeat, verticalRepeat, exifConverters.convertBytesToStringSpaceDelim(ctx, val))
		},
	}
)

func newMetaDecoderEXIF(r io.Reader, byteOrder binary.ByteOrder, thumbnailOffset int64, opts Options) *metaDecoderEXIF {
	s := newStreamReader(r, byteOrder)
	return newMetaDecoderEXIFFromStreamReader(s, thumbnailOffset, opts)
}

func newMetaDecoderEXIFFromStreamReader(s *streamReader, thumbnailOffset int64, opts Options) *metaDecoderEXIF {
	return &metaDecoderEXIF{
		thumbnailOffset: thumbnailOffset,
		seenIFDs:        map[string]struct{}{},
		streamReader:    s,
		opts:            opts,
		valueConverterCtx: valueConverterContext{
			s:         s,
			warnfFunc: opts.Warnf,
		},
	}
}

// exifType represents the basic tiff tag data types.
type exifType uint16

type metaDecoderEXIF struct {
	*streamReader
	thumbnailOffset   int64
	seenIFDs          map[string]struct{}
	valueConverterCtx valueConverterContext
	opts              Options
}

func (e *metaDecoderEXIF) convertValue(typ exifType, r io.Reader) any {
	v := e.doConvertValue(typ, r)

	switch v := v.(type) {
	case float64:
		if isUndefined(v) {
			return undef
		}
	case float32:
		if isUndefined(float64(v)) {
			return undef
		}

	}

	return v
}

func (e *metaDecoderEXIF) doConvertValue(typ exifType, r io.Reader) any {
	switch typ {
	case exifTypeUnsignedByte1, exifTypeUndef1, exifTypeASCIIString1, exifTypeSignedByte1:
		return e.read1r(r)
	case exifTypeUnsignedShort2, exifTypeSignedShort2:
		return e.read2r(r)
	case exifTypeUnsignedLong4:
		return e.read4r(r)
	case exifTypeUnsignedRat8:
		n, d := e.read4r(r), e.read4r(r)
		if d == 0 {
			return undef
		}
		r, err := NewRat[uint32](n, d)
		if err != nil {
			e.opts.Warnf("failed to convert rational: %v", err)
			return 0
		}
		return r
	case exifTypeSignedLong4:
		return e.read4sr(r)
	case exifTypeSignedRat8:
		n, d := e.read4sr(r), e.read4sr(r)
		r, err := NewRat[int32](n, d)
		if err != nil {
			e.opts.Warnf("failed to convert signed rational: %v", err)
			return 0
		}
		return r
	case exifTypeSignedFloat4:
		return math.Float32frombits(e.read4r(r))
	case exifTypeSignedDouble8:
		return math.Float64frombits(e.read8r(r))
	default:
		return newInvalidFormatError(fmt.Errorf("unknown EXIF type %d", typ))
	}
}

func (e *metaDecoderEXIF) convertValues(typ exifType, count, len int, r io.Reader) any {
	if count == 0 {
		return nil
	}

	if typ == exifTypeASCIIString1 {
		b := e.readBytesFromRVolatile(len, r)
		return string(trimBytesNulls(b[:count]))
	}

	if count == 1 {
		return e.convertValue(typ, r)
	}

	values := make([]any, count)
	allBytes := true
	for i := range count {
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
	e.seek(int64(ifd1Offset) + e.readerOffset)

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
	dataType := e.read2()
	count := e.read4()
	if count > 0x10000 {
		e.skip(4)
		return nil
	}

	tagName := exifFieldsAll[tagID]
	if tagName == "" {
		tagName = fmt.Sprintf("%s0x%x", UnknownPrefix, tagID)
	}

	if strings.Contains(tagName, " ") {
		// Space separated, pick first.
		parts := strings.Split(tagName, " ")
		tagName = parts[0]

	}

	ifd, isIFDPointer := exifIFDPointers[tagID]
	if isIFDPointer {
		if _, ok := e.seenIFDs[ifd]; ok {
			return nil
		}
		e.seenIFDs[ifd] = struct{}{}
	}

	typ := exifType(dataType)

	size, ok := exifTypeSize[typ]
	if !ok {
		return newInvalidFormatErrorf("unknown EXIF type %d", typ)
	}
	valLen := size * count

	if tagID == xmpMarker {
		if !e.opts.Sources.Has(XMP) {
			e.skip(4)
			return nil
		}

		valueOffset := e.read4()
		return e.preservePos(func() error {
			offset := valueOffset + uint32(e.readerOffset)
			e.seek(int64(offset))
			r, err := e.bufferedReader(int64(valLen))
			if err != nil {
				return err
			}
			defer r.Close()
			return decodeXMP(r, e.opts)
		})

	}

	if tagID == iptcMarker {
		if !e.opts.Sources.Has(IPTC) {
			e.skip(4)
			return nil
		}
		valueOffset := e.read4()
		return e.preservePos(func() error {
			offset := valueOffset + uint32(e.readerOffset)
			e.seek(int64(offset))
			r, err := e.bufferedReader(int64(valLen))
			if err != nil {
				return err
			}
			defer r.Close()
			iptcDec := newMetaDecoderIPTC(r, e.opts)
			return iptcDec.decodeRecords()
		})

	}

	// Below is EXIF
	if !e.opts.Sources.Has(EXIF) || valLen > uint32(e.opts.LimitTagSize) {
		e.skip(4)
		return nil
	}

	tagInfo := TagInfo{
		Source:    EXIF,
		Tag:       tagName,
		Namespace: namespace,
	}

	if !isIFDPointer && !e.opts.ShouldHandleTag(tagInfo) {
		e.skip(4)
		return nil
	}

	var val any

	if err := func() error {
		var r io.Reader = e.r
		if valLen > 4 {
			valueOffset := e.read4()
			offset := valueOffset + uint32(e.readerOffset)
			oldPos := e.pos()
			defer e.seek(oldPos)
			e.seek(int64(offset))
			rc, err := e.bufferedReader(int64(valLen))
			if err != nil {
				return err
			}
			r = rc
			defer rc.Close()
		}

		val = e.convertValues(typ, int(count), int(valLen), r)

		if valLen <= 4 {
			padding := 4 - valLen
			if padding > 0 {
				e.skip(int64(padding))
			}
		}
		return nil
	}(); err != nil {
		return err
	}

	if isIFDPointer {
		offset, ok := val.(uint32)
		if !ok {
			return newInvalidFormatErrorf("invalid IFD pointer value")
		}
		namespace := path.Join(namespace, ifd)
		return e.decodeTagsAt(namespace, int64(offset))
	}

	if convert, found := exifValueConverterMap[tagName]; found {
		e.valueConverterCtx.tagName = tagName
		val = convert(e.valueConverterCtx, val)
		if f, ok := val.(float64); ok && isUndefined(f) {
			val = undef
		}
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

	for range int(numTags) {
		if err := e.decodeTag(namespace); err != nil {
			return err
		}
	}

	return nil
}

func (e *metaDecoderEXIF) decodeTagsAt(namespace string, offset int64) error {
	return e.preservePos(
		func() error {
			e.seek(offset + e.readerOffset)
			return e.decodeTags(namespace)
		})
}

type valueConverterContext struct {
	tagName   string
	s         *streamReader
	warnfFunc func(string, ...any)
}

func (ctx valueConverterContext) warnf(format string, args ...any) {
	format = ctx.tagName + ": " + format
	ctx.warnfFunc(format, args...)
}

type valueConverter func(valueConverterContext, any) any

func init() {
	maps.Copy(exifFieldsAll, exifFields)
	maps.Copy(exifFieldsAll, exifFieldsGPS)

	for k := range exifFieldsAll {
		if k > maxEXIFField {
			maxEXIFField = k
		}
	}
}
