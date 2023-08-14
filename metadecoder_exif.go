package imagemeta

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"strings"
)

const (
	// TODO1 check usage.
	markerSOI             = 0xffd8
	markerAPP1            = 0xffe1
	markerApp13           = 0xffed
	exifPointer           = 0x8769
	gpsPointer            = 0x8825
	interopPointer        = 0xa005
	exifHeader            = 0x45786966
	pngExifMarker         = 0x65584966
	byteOrderBigEndian    = 0x4d4d
	byteOrderLittleEndian = 0x4949
	tagOrientation        = 0x0112
	tagDate               = 0x0132
)

const (
	typeUnsignedByte  exifType = 1
	typeUnsignedASCII exifType = 2
	typeUnsignedShort exifType = 3
	typeUnsignedLong  exifType = 4
	typeUnsignedRat   exifType = 5
	typeSignedByte    exifType = 6
	typeUndef         exifType = 7
	typeSignedShort   exifType = 8
	typeSignedLong    exifType = 9
	typeSignedRat     exifType = 10
	typeSignedFloat   exifType = 11
	typeSignedDouble  exifType = 12
)

// Size in bytes of each type.
var typeSize = map[exifType]uint32{
	typeUnsignedByte:  1,
	typeUnsignedASCII: 1,
	typeUnsignedShort: 2,
	typeUnsignedLong:  4,
	typeUnsignedRat:   8,
	typeSignedByte:    1,
	typeUndef:         1,
	typeSignedShort:   2,
	typeSignedLong:    4,
	typeSignedRat:     8,
	typeSignedFloat:   4,
	typeSignedDouble:  8,
}

// UnknownPrefix is used as prefix for unknown tags.
const UnknownPrefix = "UnknownTag_"

var (
	fieldsExif      = map[uint16]string{0x100: "ImageWidth", 0x101: "ImageLength", 0x102: "BitsPerSample", 0x103: "Compression", 0x106: "PhotometricInterpretation", 0x10e: "ImageDescription", 0x10f: "Make", 0x110: "Model", 0x112: "Orientation", 0x115: "SamplesPerPixel", 0x11a: "XResolution", 0x11b: "YResolution", 0x11c: "PlanarConfiguration", 0x128: "ResolutionUnit", 0x131: "Software", 0x132: "DateTime", 0x13b: "Artist", 0x212: "YCbCrSubSampling", 0x213: "YCbCrPositioning", 0x8298: "Copyright", 0x829a: "ExposureTime", 0x829d: "FNumber", 0x8769: "ExifIFDPointer", 0x8822: "ExposureProgram", 0x8824: "SpectralSensitivity", 0x8825: "GPSInfoIFDPointer", 0x8827: "ISOSpeedRatings", 0x8828: "OECF", 0x9000: "ExifVersion", 0x9003: "DateTimeOriginal", 0x9004: "DateTimeDigitized", 0x9101: "ComponentsConfiguration", 0x9102: "CompressedBitsPerPixel", 0x9201: "ShutterSpeedValue", 0x9202: "ApertureValue", 0x9203: "BrightnessValue", 0x9204: "ExposureBiasValue", 0x9205: "MaxApertureValue", 0x9206: "SubjectDistance", 0x9207: "MeteringMode", 0x9208: "LightSource", 0x9209: "Flash", 0x920a: "FocalLength", 0x9214: "SubjectArea", 0x927c: "MakerNote", 0x9286: "UserComment", 0x9290: "SubSecTime", 0x9291: "SubSecTimeOriginal", 0x9292: "SubSecTimeDigitized", 0x9c9b: "XPTitle", 0x9c9c: "XPComment", 0x9c9d: "XPAuthor", 0x9c9e: "XPKeywords", 0x9c9f: "XPSubject", 0xa000: "FlashpixVersion", 0xa001: "ColorSpace", 0xa002: "PixelXDimension", 0xa003: "PixelYDimension", 0xa004: "RelatedSoundFile", 0xa005: "InteroperabilityIFDPointer", 0xa20b: "FlashEnergy", 0xa20c: "SpatialFrequencyResponse", 0xa20e: "FocalPlaneXResolution", 0xa20f: "FocalPlaneYResolution", 0xa210: "FocalPlaneResolutionUnit", 0xa214: "SubjectLocation", 0xa215: "ExposureIndex", 0xa217: "SensingMethod", 0xa300: "FileSource", 0xa301: "SceneType", 0xa302: "CFAPattern", 0xa401: "CustomRendered", 0xa402: "ExposureMode", 0xa403: "WhiteBalance", 0xa404: "DigitalZoomRatio", 0xa405: "FocalLengthIn35mmFilm", 0xa406: "SceneCaptureType", 0xa407: "GainControl", 0xa408: "Contrast", 0xa409: "Saturation", 0xa40a: "Sharpness", 0xa40b: "DeviceSettingDescription", 0xa40c: "SubjectDistanceRange", 0xa420: "ImageUniqueID", 0xa433: "LensMake", 0xa434: "LensModel"}
	fieldsGps       = map[uint16]string{0x0: "GPSVersionID", 0x1: "GPSLatitudeRef", 0x2: "GPSLatitude", 0x3: "GPSLongitudeRef", 0x4: "GPSLongitude", 0x5: "GPSAltitudeRef", 0x6: "GPSAltitude", 0x7: "GPSTimeStamp", 0x8: "GPSSatelites", 0x9: "GPSStatus", 0xa: "GPSMeasureMode", 0xb: "GPSDOP", 0xc: "GPSSpeedRef", 0xd: "GPSSpeed", 0xe: "GPSTrackRef", 0xf: "GPSTrack", 0x10: "GPSImgDirectionRef", 0x11: "GPSImgDirection", 0x12: "GPSMapDatum", 0x13: "GPSDestLatitudeRef", 0x14: "GPSDestLatitude", 0x15: "GPSDestLongitudeRef", 0x16: "GPSDestLongitude", 0x17: "GPSDestBearingRef", 0x18: "GPSDestBearing", 0x19: "GPSDestDistanceRef", 0x1a: "GPSDestDistance", 0x1b: "GPSProcessingMethod", 0x1c: "GPSAreaInformation", 0x1d: "GPSDateStamp", 0x1e: "GPSDifferential"}
	fieldsInterop   = map[uint16]string{0x1: "InteroperabilityIndex"}
	fieldsThumbnail = map[uint16]string{0x201: "ThumbJPEGInterchangeFormat", 0x202: "ThumbJPEGInterchangeFormatLength"}

	fieldsAll = map[uint16]string{}
)

func newMetaDecoderEXIF(r io.Reader, handleTag HandleTagFunc) *metaDecoderEXIF {
	return &metaDecoderEXIF{
		streamReader: newStreamReader(r),
		handleTag:    handleTag,
	}
}

type metaDecoderEXIF struct {
	*streamReader
	handleTag HandleTagFunc
}

func (e *metaDecoderEXIF) convertValue(typ exifType, r io.Reader) any {
	switch typ {
	case typeUnsignedByte, typeUndef:
		return e.read1r(r)
	case typeUnsignedShort:
		return e.read2r(r)
	case typeUnsignedLong:
		return e.read4r(r)
	case typeSignedLong:
		return e.read4sr(r)
	case typeUnsignedRat:
		n, d := e.read4r(r), e.read4r(r)
		return big.NewRat(int64(n), int64(d))
	case typeSignedRat:
		n, d := e.read4sr(r), e.read4sr(r)
		return big.NewRat(int64(n), int64(d))
	default:
		// TODO1
		panic(fmt.Errorf("exif type %d not implemented", typ))
	}
}

func (e *metaDecoderEXIF) convertValues(typ exifType, count, len int, r io.Reader) any {
	if count == 0 {
		return nil
	}

	if typ == typeUnsignedASCII {
		buff := getBuffer()
		defer putBuffer(buff)
		// Read len bytes into buff from r.
		n, err := io.CopyN(buff, r, int64(len))
		if err != nil || n != int64(len) {
			// TODO1
			panic(err)
		}
		return string(buff.Bytes()[:count-1])
	}

	if count == 1 {
		return e.convertValue(typ, r)
	}

	values := make([]any, count)
	for i := 0; i < count; i++ {
		values[i] = e.convertValue(typ, r)
	}
	return values
}

func (e *metaDecoderEXIF) decode() (err error) {
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

	firstOffset := e.read4()

	if firstOffset < 8 {
		return nil
	}

	e.skip(int64(firstOffset - 8))
	e.readerOffset = e.pos() - 8

	return e.decodeTags()

}

func (e *metaDecoderEXIF) decodeTags() error {
	if e.done() {
		e.stop(nil)
	}

	numTags := e.read2()

	for i := 0; i < int(numTags); i++ {
		if err := e.decodeTag(); err != nil {
			return err
		}
	}

	return nil
}

func (e *metaDecoderEXIF) decodeTagsAt(offset int) error {
	oldPos := e.pos()
	defer func() {
		e.seek(oldPos)
	}()
	e.seek(offset + e.readerOffset)
	return e.decodeTags()
}

func (e *metaDecoderEXIF) done() bool {
	return false // TODO1
}

func init() {
	for k, v := range fieldsExif {
		fieldsAll[k] = v
	}
	for k, v := range fieldsGps {
		fieldsAll[k] = v
	}
	for k, v := range fieldsInterop {
		fieldsAll[k] = v
	}
	for k, v := range fieldsThumbnail {
		fieldsAll[k] = v
	}
}

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
		return ErrInvalidFormat
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
		return e.decodeTagsAt(int(offset))
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
